package model

import "time"

// OwnershipTransfer 是一次归属变更动作的留痕，只追加、不修改。
type OwnershipTransfer struct {
	ID            int64     `db:"id" json:"id"`
	PlotID        int64     `db:"plot_id" json:"plot_id"`
	FromFarmID    int64     `db:"from_farm_id" json:"from_farm_id"`
	ToFarmID      int64     `db:"to_farm_id" json:"to_farm_id"`
	FromFarmName  string    `db:"from_farm_name" json:"from_farm_name,omitempty"`
	ToFarmName    string    `db:"to_farm_name" json:"to_farm_name,omitempty"`
	Reason        string    `db:"reason" json:"reason"`
	Note          string    `db:"note" json:"note,omitempty"`
	Operator      string    `db:"operator" json:"operator"`
	BatchCount    int       `db:"batch_count" json:"batch_count"`
	CodeCount     int       `db:"code_count" json:"code_count"`
	TransferredAt time.Time `db:"transferred_at" json:"transferred_at"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
}

// OwnershipPeriod 是归属时间线上的一段 [ValidFrom, ValidTo)，ValidTo 为空表示当前。
type OwnershipPeriod struct {
	ID         int64      `db:"id" json:"id"`
	PlotID     int64      `db:"plot_id" json:"plot_id"`
	FarmID     int64      `db:"farm_id" json:"farm_id"`
	FarmName   string     `db:"farm_name" json:"farm_name,omitempty"`
	ValidFrom  time.Time  `db:"valid_from" json:"valid_from"`
	ValidTo    *time.Time `db:"valid_to" json:"valid_to,omitempty"`
	TransferID *int64     `db:"transfer_id" json:"transfer_id,omitempty"`
	CreatedAt  time.Time  `db:"created_at" json:"created_at"`
}

type TransferReason string

const (
	TransferReasonTransfer TransferReason = "transfer" // 整块地转给邻村/他人
	TransferReasonMerge    TransferReason = "merge"    // 合作社并入
	TransferReasonSplit    TransferReason = "split"    // 拆分
)

type TransferOwnershipRequest struct {
	ToFarmID int64  `json:"to_farm_id" binding:"required"`
	Reason   string `json:"reason"`
	Note     string `json:"note"`
	Operator string `json:"operator" binding:"required"`
	// 名下仍挂着批次时，必须显式确认才允许继续变更。
	ConfirmHangingBatches bool `json:"confirm_hanging_batches"`
}

type BatchBrief struct {
	ID     int64       `db:"id" json:"id"`
	CropID string      `db:"crop_id" json:"crop_id"`
	Status BatchStatus `db:"status" json:"status"`
}

// OwnershipPrecheck 是变更前的提示信息。
type OwnershipPrecheck struct {
	PlotID           int64        `json:"plot_id"`
	PlotName         string       `json:"plot_name"`
	CurrentFarmID    int64        `json:"current_farm_id"`
	CurrentFarmName  string       `json:"current_farm_name"`
	ToFarmID         int64        `json:"to_farm_id"`
	ToFarmName       string       `json:"to_farm_name,omitempty"`
	TotalBatchCount  int          `json:"total_batch_count"`
	ActiveBatchCount int          `json:"active_batch_count"`
	IssuedCodeCount  int          `json:"issued_code_count"`
	ActiveBatches    []BatchBrief `json:"active_batches"`
	CanTransfer      bool         `json:"can_transfer"`
	Warnings         []string     `json:"warnings"`
}
