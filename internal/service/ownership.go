package service

import (
	"cc-052/internal/model"
	"cc-052/internal/repository"
	"time"
)

// TransferNeedsConfirm 地块上仍挂有批次，需调用方确认后带 confirm=true 重试。
type TransferNeedsConfirm struct {
	Warning *model.TransferWarning
}

func (e *TransferNeedsConfirm) Error() string {
	return "转出方名下仍挂有批次，请先确认"
}

// 提示里最多带出的批次条数，避免批次很多时响应过大
const transferWarningBatchLimit = 20

type OwnershipService struct {
	repo      *repository.OwnershipRepo
	batchRepo *repository.BatchRepo
	plotRepo  *repository.PlotRepo
	farmRepo  *repository.FarmRepo
}

func NewOwnershipService(
	repo *repository.OwnershipRepo,
	batchRepo *repository.BatchRepo,
	plotRepo *repository.PlotRepo,
	farmRepo *repository.FarmRepo,
) *OwnershipService {
	return &OwnershipService{repo: repo, batchRepo: batchRepo, plotRepo: plotRepo, farmRepo: farmRepo}
}

func (s *OwnershipService) Transfer(plotID int64, req *model.TransferPlotRequest) (*model.TransferInfo, error) {
	if req.FromFarmID == req.ToFarmID {
		return nil, model.ErrSameFarm
	}
	// 转出方名下还挂着批次 → 先提示，confirm=true 才放行
	if !req.Confirm {
		count, err := s.batchRepo.CountByPlot(plotID)
		if err != nil {
			return nil, err
		}
		if count > 0 {
			batches, err := s.batchRepo.ListByPlot(plotID, transferWarningBatchLimit)
			if err != nil {
				return nil, err
			}
			return nil, &TransferNeedsConfirm{Warning: &model.TransferWarning{
				BatchCount: count,
				Batches:    batches,
				Hint:       "变更后历史批次与已发出的溯源码仍按当时归属回放，不受变更影响；确认继续请重新提交并带 confirm=true",
			}}
		}
	}
	return s.repo.Transfer(plotID, req)
}

// History 一块地的完整归属档案：变更前后各自挂在谁名下 + 每次变更动作。
func (s *OwnershipService) History(plotID int64) (*model.OwnershipHistory, error) {
	plot, err := s.plotRepo.GetByID(plotID)
	if err != nil {
		return nil, model.ErrPlotNotFound
	}
	periods, err := s.repo.ListPeriods(plotID)
	if err != nil {
		return nil, err
	}
	transfers, err := s.repo.ListTransfers(plotID)
	if err != nil {
		return nil, err
	}
	return &model.OwnershipHistory{
		PlotID:        plot.ID,
		CurrentFarmID: plot.FarmID,
		Periods:       periods,
		Transfers:     transfers,
	}, nil
}

// OwnerAt 回放指定时刻的归属；时刻早于首条归属期时回退到当前归属。
func (s *OwnershipService) OwnerAt(plotID int64, t time.Time) (*model.OwnerAtResponse, error) {
	plot, err := s.plotRepo.GetByID(plotID)
	if err != nil {
		return nil, model.ErrPlotNotFound
	}
	farmID, err := s.repo.OwnerAt(plotID, t)
	if err != nil {
		return nil, err
	}
	if farmID == 0 {
		farmID = plot.FarmID
	}
	farm, err := s.farmRepo.GetByID(farmID)
	if err != nil {
		return nil, err
	}
	return &model.OwnerAtResponse{
		PlotID:   plot.ID,
		FarmID:   farm.ID,
		FarmName: farm.Name,
		AsOf:     t,
	}, nil
}
