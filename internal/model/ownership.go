package model

import "time"

// TransferPlotRequest 地块归属变更请求。
// FromFarmID 必须等于地块当前归属（compare-and-swap），防止两家同时把一块地认作自己的；
// 地块上仍挂有批次时需 Confirm=true 才会执行（先提示、确认后放行）。
type TransferPlotRequest struct {
	FromFarmID int64  `json:"from_farm_id" binding:"required"`
	ToFarmID   int64  `json:"to_farm_id" binding:"required"`
	Reason     string `json:"reason"`
	Operator   string `json:"operator" binding:"required"`
	Confirm    bool   `json:"confirm"`
}

// TransferInfo 一次归属变更动作（不可变记录，可回看）。
type TransferInfo struct {
	ID           int64     `db:"id" json:"id"`
	PlotID       int64     `db:"plot_id" json:"plot_id"`
	FromFarmID   int64     `db:"from_farm_id" json:"from_farm_id"`
	FromFarmName string    `db:"from_farm_name" json:"from_farm_name"`
	ToFarmID     int64     `db:"to_farm_id" json:"to_farm_id"`
	ToFarmName   string    `db:"to_farm_name" json:"to_farm_name"`
	Reason       string    `db:"reason" json:"reason"`
	Operator     string    `db:"operator" json:"operator"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

// OwnershipPeriodInfo 一段归属期 [ValidFrom, ValidTo)；ValidTo 为空表示当前归属。
type OwnershipPeriodInfo struct {
	ID         int64      `db:"id" json:"id"`
	PlotID     int64      `db:"plot_id" json:"plot_id"`
	FarmID     int64      `db:"farm_id" json:"farm_id"`
	FarmName   string     `db:"farm_name" json:"farm_name"`
	ValidFrom  time.Time  `db:"valid_from" json:"valid_from"`
	ValidTo    *time.Time `db:"valid_to" json:"valid_to,omitempty"`
	TransferID *int64     `db:"transfer_id" json:"transfer_id,omitempty"`
}

// OwnershipHistory 一块地的完整归属档案：变更前后各自挂在谁名下 + 每次变更动作。
type OwnershipHistory struct {
	PlotID        int64                 `json:"plot_id"`
	CurrentFarmID int64                 `json:"current_farm_id"`
	Periods       []OwnershipPeriodInfo `json:"periods"`
	Transfers     []TransferInfo        `json:"transfers"`
}

// OwnerAtResponse 指定时刻的归属（历史回放）。
type OwnerAtResponse struct {
	PlotID   int64     `json:"plot_id"`
	FarmID   int64     `json:"farm_id"`
	FarmName string    `json:"farm_name"`
	AsOf     time.Time `json:"as_of"`
}

// TransferWarning 转出方名下仍挂有批次时的提示内容。
type TransferWarning struct {
	BatchCount int         `json:"batch_count"`
	Batches    []CropBatch `json:"batches"`
	Hint       string      `json:"hint"`
}
