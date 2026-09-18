package repository

import (
	"cc-052/internal/model"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

type OwnershipRepo struct {
	db *sqlx.DB
}

func NewOwnershipRepo(db *sqlx.DB) *OwnershipRepo {
	return &OwnershipRepo{db: db}
}

// Transfer 在单个事务里完成归属变更：
// 锁地块行（串行化并发变更）→ 校验当前归属 → 写变更动作 → 关闭旧归属期 → 开启新归属期 → 更新当前归属。
// 变更以记录时间生效，不允许回填过去时间——否则会改写已发出溯源码的对外说法。
func (r *OwnershipRepo) Transfer(plotID int64, req *model.TransferPlotRequest) (*model.TransferInfo, error) {
	tx, err := r.db.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// FOR UPDATE 锁住地块行：并发的两次变更在此排队，后到者看到的新归属与
	// 自己声称的 from_farm_id 对不上，被 ErrOwnerMismatch 拦下
	var currentFarmID int64
	if err := tx.Get(&currentFarmID, `SELECT farm_id FROM plot WHERE id = $1 FOR UPDATE`, plotID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrPlotNotFound
		}
		return nil, err
	}
	if currentFarmID == req.ToFarmID {
		return nil, model.ErrSameFarm
	}
	if currentFarmID != req.FromFarmID {
		return nil, model.ErrOwnerMismatch
	}

	var toFarmExists bool
	if err := tx.Get(&toFarmExists, `SELECT EXISTS(SELECT 1 FROM farm WHERE id = $1)`, req.ToFarmID); err != nil {
		return nil, err
	}
	if !toFarmExists {
		return nil, model.ErrFarmNotFound
	}

	now := time.Now()

	// 1) 变更动作记录（一次可回看的动作）
	var transferID int64
	if err := tx.QueryRowx(`INSERT INTO plot_transfer (plot_id, from_farm_id, to_farm_id, reason, operator)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		plotID, req.FromFarmID, req.ToFarmID, req.Reason, req.Operator).Scan(&transferID); err != nil {
		return nil, err
	}

	// 2) 关闭当前归属期
	if _, err := tx.Exec(`UPDATE plot_ownership SET valid_to = $1 WHERE plot_id = $2 AND valid_to IS NULL`, now, plotID); err != nil {
		return nil, err
	}

	// 3) 开启新归属期（idx_plot_ownership_current 部分唯一索引兜底：一块地最多一条有效归属期）
	if _, err := tx.Exec(`INSERT INTO plot_ownership (plot_id, farm_id, valid_from, valid_to, transfer_id)
		VALUES ($1, $2, $3, NULL, $4)`, plotID, req.ToFarmID, now, transferID); err != nil {
		return nil, err
	}

	// 4) 更新当前归属
	if _, err := tx.Exec(`UPDATE plot SET farm_id = $1 WHERE id = $2`, req.ToFarmID, plotID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.GetTransfer(transferID)
}

// GetTransfer 按 ID 取变更动作（含双方农场名）。
func (r *OwnershipRepo) GetTransfer(id int64) (*model.TransferInfo, error) {
	var t model.TransferInfo
	query := `SELECT t.id, t.plot_id, t.from_farm_id, ff.name AS from_farm_name,
	                 t.to_farm_id, tf.name AS to_farm_name, t.reason, t.operator, t.created_at
	          FROM plot_transfer t
	          JOIN farm ff ON ff.id = t.from_farm_id
	          JOIN farm tf ON tf.id = t.to_farm_id
	          WHERE t.id = $1`
	if err := r.db.Get(&t, query, id); err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTransfers 一块地的全部变更动作，按时间升序。
func (r *OwnershipRepo) ListTransfers(plotID int64) ([]model.TransferInfo, error) {
	transfers := make([]model.TransferInfo, 0)
	query := `SELECT t.id, t.plot_id, t.from_farm_id, ff.name AS from_farm_name,
	                 t.to_farm_id, tf.name AS to_farm_name, t.reason, t.operator, t.created_at
	          FROM plot_transfer t
	          JOIN farm ff ON ff.id = t.from_farm_id
	          JOIN farm tf ON tf.id = t.to_farm_id
	          WHERE t.plot_id = $1 ORDER BY t.id`
	if err := r.db.Select(&transfers, query, plotID); err != nil {
		return nil, err
	}
	return transfers, nil
}

// ListPeriods 一块地的全部归属期，按生效时间升序。
func (r *OwnershipRepo) ListPeriods(plotID int64) ([]model.OwnershipPeriodInfo, error) {
	periods := make([]model.OwnershipPeriodInfo, 0)
	query := `SELECT po.id, po.plot_id, po.farm_id, f.name AS farm_name,
	                 po.valid_from, po.valid_to, po.transfer_id
	          FROM plot_ownership po
	          JOIN farm f ON f.id = po.farm_id
	          WHERE po.plot_id = $1 ORDER BY po.valid_from, po.id`
	if err := r.db.Select(&periods, query, plotID); err != nil {
		return nil, err
	}
	return periods, nil
}

// OwnerAt 回放：时刻 t 这块地挂在谁名下。查不到（t 早于首条归属期）时返回 0，由调用方回退。
func (r *OwnershipRepo) OwnerAt(plotID int64, t time.Time) (int64, error) {
	var farmID int64
	err := r.db.Get(&farmID, `SELECT farm_id FROM plot_ownership
		WHERE plot_id = $1 AND valid_from <= $2 AND (valid_to IS NULL OR valid_to > $2)
		ORDER BY valid_from DESC, id DESC LIMIT 1`, plotID, t)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return farmID, nil
}
