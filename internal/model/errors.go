package model

import "errors"

// 归属变更领域的哨兵错误，handler 用 errors.Is 映射到 HTTP 状态码。
var (
	ErrPlotNotFound  = errors.New("plot not found")
	ErrFarmNotFound  = errors.New("farm not found")
	ErrOwnerMismatch = errors.New("from_farm_id 与地块当前归属不一致，地块可能已被他人变更")
	ErrSameFarm      = errors.New("转入方与当前归属相同，无需变更")
)
