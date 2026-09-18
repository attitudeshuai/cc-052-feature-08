package repository

import (
	"cc-052/internal/model"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

var (
	// ErrOwnershipConflict 表示变更提交时该地块当前归属已不是预期的转出方，
	// 通常是并发变更导致，也用来拦住"两家同时把一块地认作自己的"。
	ErrOwnershipConflict = errors.New("plot ownership conflict: current owner changed or already claimed")
	// ErrNoCurrentOwnership 表示地块还没有任何归属段。
	ErrNoCurrentOwnership = errors.New("plot has no current ownership period")
)

type OwnershipRepo struct {
	db *sqlx.DB
}

func NewOwnershipRepo(db *sqlx.DB) *OwnershipRepo {
	return &OwnershipRepo{db: db}
}

const periodCols = `p.id, p.plot_id, p.farm_id, f.name AS farm_name,
	p.valid_from, p.valid_to, p.transfer_id, p.created_at`

// CurrentOwnership 返回地块当前归属段（valid_to 为空）。
func (r *OwnershipRepo) CurrentOwnership(plotID int64) (*model.OwnershipPeriod, error) {
	var p model.OwnershipPeriod
	q := `SELECT ` + periodCols + `
	      FROM plot_ownership_period p JOIN farm f ON f.id = p.farm_id
	      WHERE p.plot_id = $1 AND p.valid_to IS NULL`
	if err := r.db.Get(&p, q, plotID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoCurrentOwnership
		}
		return nil, err
	}
	return &p, nil
}

// OwnershipAt 返回地块在 at 时刻的归属段，用于历史回放。
func (r *OwnershipRepo) OwnershipAt(plotID int64, at time.Time) (*model.OwnershipPeriod, error) {
	var p model.OwnershipPeriod
	q := `SELECT ` + periodCols + `
	      FROM plot_ownership_period p JOIN farm f ON f.id = p.farm_id
	      WHERE p.plot_id = $1 AND p.valid_from <= $2
	        AND (p.valid_to IS NULL OR p.valid_to > $2)
	      ORDER BY p.valid_from DESC LIMIT 1`
	if err := r.db.Get(&p, q, plotID, at); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoCurrentOwnership
		}
		return nil, err
	}
	return &p, nil
}

// CountBatchesByPlot 返回该地块名下的批次总数与仍在挂（未锁定）批次数。
func (r *OwnershipRepo) CountBatchesByPlot(plotID int64) (total int, active int, err error) {
	q := `SELECT
	          COUNT(*) AS total,
	          COUNT(*) FILTER (WHERE status <> 'locked') AS active
	      FROM crop_batch WHERE plot_id = $1`
	row := struct {
		Total  int `db:"total"`
		Active int `db:"active"`
	}{}
	if err = r.db.Get(&row, q, plotID); err != nil {
		return 0, 0, err
	}
	return row.Total, row.Active, nil
}

// ListActiveBatchesByPlot 列出仍在挂的批次，供变更前提示。
func (r *OwnershipRepo) ListActiveBatchesByPlot(plotID int64) ([]model.BatchBrief, error) {
	var bs []model.BatchBrief
	q := `SELECT id, crop_id, status FROM crop_batch
	      WHERE plot_id = $1 AND status <> 'locked' ORDER BY id`
	if err := r.db.Select(&bs, q, plotID); err != nil {
		return nil, err
	}
	return bs, nil
}

// CountCodesByPlot 统计该地块所有批次已经发出去的码数量。
func (r *OwnershipRepo) CountCodesByPlot(plotID int64) (int, error) {
	var n int
	q := `SELECT COUNT(*) FROM trace_code tc
	      JOIN crop_batch cb ON cb.id = tc.batch_id
	      WHERE cb.plot_id = $1`
	if err := r.db.Get(&n, q, plotID); err != nil {
		return 0, err
	}
	return n, nil
}

// ExecTransfer 在一个事务内原子地完成一次归属变更：
// 写变更流水、闭合旧归属段、开启新归属段、更新 plot.farm_id。
// 行锁 + 归属复核保证并发下不会把同一块地同时认给两家。
func (r *OwnershipRepo) ExecTransfer(
	plotID, fromFarmID, toFarmID int64,
	reason, note, operator string,
	batchCount, codeCount int,
	at time.Time,
) (*model.OwnershipTransfer, error) {
	tx, err := r.db.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 锁住地块行，串行化同一地块上的并发变更。
	var lockedFarmID int64
	if err := tx.Get(&lockedFarmID,
		`SELECT farm_id FROM plot WHERE id = $1 FOR UPDATE`, plotID); err != nil {
		return nil, err
	}
	if lockedFarmID != fromFarmID {
		return nil, ErrOwnershipConflict
	}

	// 锁住当前归属段并复核转出方，防止区间被并发改动。
	var currentPeriodID int64
	err = tx.Get(&currentPeriodID,
		`SELECT id FROM plot_ownership_period
		 WHERE plot_id = $1 AND valid_to IS NULL AND farm_id = $2
		 FOR UPDATE`, plotID, fromFarmID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOwnershipConflict
		}
		return nil, err
	}

	t := &model.OwnershipTransfer{
		PlotID:        plotID,
		FromFarmID:    fromFarmID,
		ToFarmID:      toFarmID,
		Reason:        reason,
		Note:          note,
		Operator:      operator,
		BatchCount:    batchCount,
		CodeCount:     codeCount,
		TransferredAt: at,
	}
	if t.Reason == "" {
		t.Reason = string(model.TransferReasonTransfer)
	}

	if err := tx.QueryRow(
		`INSERT INTO plot_ownership_transfer
		 (plot_id, from_farm_id, to_farm_id, reason, note, operator, batch_count, code_count, transferred_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id, created_at`,
		t.PlotID, t.FromFarmID, t.ToFarmID, t.Reason, t.Note, t.Operator,
		t.BatchCount, t.CodeCount, t.TransferredAt,
	).Scan(&t.ID, &t.CreatedAt); err != nil {
		return nil, err
	}

	// 闭合旧段。
	res, err := tx.Exec(
		`UPDATE plot_ownership_period SET valid_to = $1
		 WHERE id = $2 AND valid_to IS NULL`, at, currentPeriodID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, ErrOwnershipConflict
	}

	// 开启新段。唯一部分索引 uq_ownership_current 在这里兜底拦截重复认领。
	if _, err := tx.Exec(
		`INSERT INTO plot_ownership_period (plot_id, farm_id, valid_from, transfer_id)
		 VALUES ($1,$2,$3,$4)`, plotID, toFarmID, at, t.ID); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return nil, ErrOwnershipConflict
		}
		return nil, err
	}

	// 更新地块当前归属，保持与时间线最新段一致。
	if _, err := tx.Exec(`UPDATE plot SET farm_id = $1 WHERE id = $2`, toFarmID, plotID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return t, nil
}

// ListTransfers 列出某地块（或全部，plotID=0）的归属变更动作，含双方名称。
func (r *OwnershipRepo) ListTransfers(plotID int64) ([]model.OwnershipTransfer, error) {
	var ts []model.OwnershipTransfer
	q := `SELECT t.id, t.plot_id, t.from_farm_id, t.to_farm_id,
	             ff.name AS from_farm_name, tf.name AS to_farm_name,
	             t.reason, t.note, t.operator, t.batch_count, t.code_count,
	             t.transferred_at, t.created_at
	      FROM plot_ownership_transfer t
	      JOIN farm ff ON ff.id = t.from_farm_id
	      JOIN farm tf ON tf.id = t.to_farm_id`
	if plotID > 0 {
		q += ` WHERE t.plot_id = $1 ORDER BY t.transferred_at DESC, t.id DESC`
		if err := r.db.Select(&ts, q, plotID); err != nil {
			return nil, err
		}
	} else {
		q += ` ORDER BY t.transferred_at DESC, t.id DESC`
		if err := r.db.Select(&ts, q); err != nil {
			return nil, err
		}
	}
	return ts, nil
}

// ListPeriods 列出地块归属时间线。
func (r *OwnershipRepo) ListPeriods(plotID int64) ([]model.OwnershipPeriod, error) {
	var ps []model.OwnershipPeriod
	q := `SELECT ` + periodCols + `
	      FROM plot_ownership_period p JOIN farm f ON f.id = p.farm_id
	      WHERE p.plot_id = $1 ORDER BY p.valid_from DESC`
	if err := r.db.Select(&ps, q, plotID); err != nil {
		return nil, err
	}
	return ps, nil
}
