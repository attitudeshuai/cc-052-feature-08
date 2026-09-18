package service

import (
	"cc-052/internal/model"
	"cc-052/internal/repository"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrSameFarm 转入转出是同一家，无意义变更。
	ErrSameFarm = errors.New("转入方与转出方不能是同一家")
	// ErrHangingBatches 转出方名下仍挂着批次且未显式确认。
	ErrHangingBatches = errors.New("转出方名下仍挂着批次，需先核对并显式确认")
	// ErrOwnershipConflict 地块当前归属与预期不符（并发变更/重复认领）。
	ErrOwnershipConflict = errors.New("该地块当前归属已变化或已被对方认领，变更被拦截")
)

type OwnershipService struct {
	repo     *repository.OwnershipRepo
	plotRepo *repository.PlotRepo
	farmRepo *repository.FarmRepo
}

func NewOwnershipService(
	repo *repository.OwnershipRepo,
	plotRepo *repository.PlotRepo,
	farmRepo *repository.FarmRepo,
) *OwnershipService {
	return &OwnershipService{repo: repo, plotRepo: plotRepo, farmRepo: farmRepo}
}

// Precheck 变更前预检：给出当前归属、在挂批次、已发码和提示，不做任何写操作。
func (s *OwnershipService) Precheck(plotID, toFarmID int64) (*model.OwnershipPrecheck, error) {
	plot, err := s.plotRepo.GetByID(plotID)
	if err != nil {
		return nil, fmt.Errorf("plot not found: %w", err)
	}
	current, err := s.repo.CurrentOwnership(plotID)
	if err != nil {
		return nil, err
	}
	toFarm, err := s.farmRepo.GetByID(toFarmID)
	if err != nil {
		return nil, fmt.Errorf("target farm not found: %w", err)
	}

	total, active, err := s.repo.CountBatchesByPlot(plotID)
	if err != nil {
		return nil, err
	}
	codeCount, err := s.repo.CountCodesByPlot(plotID)
	if err != nil {
		return nil, err
	}
	var activeBatches []model.BatchBrief
	if active > 0 {
		activeBatches, err = s.repo.ListActiveBatchesByPlot(plotID)
		if err != nil {
			return nil, err
		}
	}

	pc := &model.OwnershipPrecheck{
		PlotID:           plotID,
		PlotName:         plot.Name,
		CurrentFarmID:    current.FarmID,
		CurrentFarmName:  current.FarmName,
		ToFarmID:         toFarmID,
		ToFarmName:       toFarm.Name,
		TotalBatchCount:  total,
		ActiveBatchCount: active,
		IssuedCodeCount:  codeCount,
		ActiveBatches:    activeBatches,
		CanTransfer:      true,
		Warnings:         []string{},
	}

	if current.FarmID == toFarmID {
		pc.CanTransfer = false
		pc.Warnings = append(pc.Warnings, "转入方与当前归属是同一家，无需变更")
	}
	if active > 0 {
		pc.Warnings = append(pc.Warnings, fmt.Sprintf(
			"转出方名下仍挂着 %d 个未锁定批次，变更后这些批次将随地块归入新主体，请核对", active))
	}
	if codeCount > 0 {
		pc.Warnings = append(pc.Warnings, fmt.Sprintf(
			"该地块已有 %d 个对外发出的溯源码，已对外给出的归属说法不会随本次变更改变", codeCount))
	}
	return pc, nil
}

// Transfer 执行一次归属变更。
func (s *OwnershipService) Transfer(plotID int64, req *model.TransferOwnershipRequest) (*model.OwnershipTransfer, error) {
	if _, err := s.plotRepo.GetByID(plotID); err != nil {
		return nil, fmt.Errorf("plot not found: %w", err)
	}
	current, err := s.repo.CurrentOwnership(plotID)
	if err != nil {
		return nil, err
	}
	if _, err := s.farmRepo.GetByID(req.ToFarmID); err != nil {
		return nil, fmt.Errorf("target farm not found: %w", err)
	}

	// 同一家不允许变更。
	if current.FarmID == req.ToFarmID {
		return nil, ErrSameFarm
	}

	total, active, err := s.repo.CountBatchesByPlot(plotID)
	if err != nil {
		return nil, err
	}
	// 名下还挂着批次时，必须先提示并得到显式确认。
	if active > 0 && !req.ConfirmHangingBatches {
		return nil, ErrHangingBatches
	}
	codeCount, err := s.repo.CountCodesByPlot(plotID)
	if err != nil {
		return nil, err
	}

	reason := req.Reason
	if reason == "" {
		reason = string(model.TransferReasonTransfer)
	}

	at := time.Now()
	t, err := s.repo.ExecTransfer(
		plotID, current.FarmID, req.ToFarmID,
		reason, req.Note, req.Operator,
		total, codeCount, at,
	)
	if err != nil {
		if errors.Is(err, repository.ErrOwnershipConflict) {
			return nil, ErrOwnershipConflict
		}
		return nil, err
	}
	return t, nil
}

func (s *OwnershipService) ListTransfers(plotID int64) ([]model.OwnershipTransfer, error) {
	return s.repo.ListTransfers(plotID)
}

func (s *OwnershipService) ListPeriods(plotID int64) ([]model.OwnershipPeriod, error) {
	return s.repo.ListPeriods(plotID)
}

// OwnershipAt 供溯源回放使用：取地块在 at 时刻的归属。
func (s *OwnershipService) OwnershipAt(plotID int64, at time.Time) (*model.OwnershipPeriod, error) {
	return s.repo.OwnershipAt(plotID, at)
}
