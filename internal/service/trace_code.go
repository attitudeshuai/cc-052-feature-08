package service

import (
	"cc-052/internal/model"
	"cc-052/internal/repository"
	"cc-052/pkg/tracecode"
	"fmt"
	"time"
)

type TraceCodeService struct {
	codeRepo       *repository.TraceCodeRepo
	batchRepo      *repository.BatchRepo
	inspectionRepo *repository.InspectionRepo
	activityRepo   *repository.ActivityRepo
	plotRepo       *repository.PlotRepo
	farmRepo       *repository.FarmRepo
	ownershipRepo  *repository.OwnershipRepo
}

func NewTraceCodeService(
	codeRepo *repository.TraceCodeRepo,
	batchRepo *repository.BatchRepo,
	inspectionRepo *repository.InspectionRepo,
	activityRepo *repository.ActivityRepo,
	plotRepo *repository.PlotRepo,
	farmRepo *repository.FarmRepo,
	ownershipRepo *repository.OwnershipRepo,
) *TraceCodeService {
	return &TraceCodeService{
		codeRepo:       codeRepo,
		batchRepo:      batchRepo,
		inspectionRepo: inspectionRepo,
		activityRepo:   activityRepo,
		plotRepo:       plotRepo,
		farmRepo:       farmRepo,
		ownershipRepo:  ownershipRepo,
	}
}

func (s *TraceCodeService) GenerateCodes(batchID int64, count int) ([]string, error) {
	// Check batch exists
	batch, err := s.batchRepo.GetByID(batchID)
	if err != nil {
		return nil, fmt.Errorf("batch not found: %w", err)
	}

	// Check if batch is locked
	if batch.Status == model.BatchStatusLocked {
		return nil, fmt.Errorf("batch is locked, cannot generate codes")
	}

	// Check inspection - must have passed
	passed, err := s.inspectionRepo.HasPassedInspection(batchID)
	if err != nil || !passed {
		return nil, fmt.Errorf("batch has not passed inspection")
	}

	// Check safety interval
	ok, msg := s.checkSafetyInterval(batch)
	if !ok {
		return nil, fmt.Errorf("safety interval check failed: %s", msg)
	}

	// Get max seq
	maxSeq, err := s.codeRepo.GetMaxSeqByBatch(batchID)
	if err != nil {
		return nil, err
	}

	// Generate codes in batch of 1000
	var allCodes []string
	batchSize := 1000
	for i := 0; i < count; i += batchSize {
		end := i + batchSize
		if end > count {
			end = count
		}
		size := end - i

		var codes []model.TraceCode
		var codeStrings []string
		for j := 0; j < size; j++ {
			seq := int64(maxSeq + i + j + 1)
			code := tracecode.Generate(seq)
			codes = append(codes, model.TraceCode{
				BatchID: batchID,
				Code:    code,
				Seq:     int(seq),
			})
			codeStrings = append(codeStrings, code)
		}

		if err := s.codeRepo.BatchInsert(codes); err != nil {
			return nil, fmt.Errorf("batch insert codes: %w", err)
		}
		allCodes = append(allCodes, codeStrings...)
	}

	return allCodes, nil
}

func (s *TraceCodeService) Trace(code string, region string, asOf *time.Time) (*model.TraceResponse, error) {
	tc, err := s.codeRepo.GetByCode(code)
	if err != nil {
		return nil, fmt.Errorf("code not found: %w", err)
	}

	// 确定"对外说法"的归属时点：
	//  - 指定 as_of 回放：按该时点取归属，且不改变首扫状态；
	//  - 已首扫的码：说法冻结在首扫时刻，地块之后再转手也不变；
	//  - 首次扫描：以当前归属定格，并记录首扫时间。
	isFirstScan := tc.FirstScannedAt == nil && asOf == nil
	var anchor time.Time
	switch {
	case asOf != nil:
		anchor = *asOf
	case tc.FirstScannedAt != nil:
		anchor = *tc.FirstScannedAt
	default:
		anchor = time.Now()
	}
	if isFirstScan {
		s.codeRepo.MarkScanned(tc.ID, region, anchor)
	}

	batch, err := s.batchRepo.GetByID(tc.BatchID)
	if err != nil {
		return nil, err
	}

	plot, err := s.plotRepo.GetByID(batch.PlotID)
	if err != nil {
		return nil, err
	}

	// 按归属时点回放：取地块在 anchor 时刻挂在哪家名下。
	period, perr := s.ownershipRepo.OwnershipAt(plot.ID, anchor)
	var farmID int64
	if perr != nil {
		// 兜底：理论上迁移已为每块地从创建时回填归属段，不会走到这里。
		farmID = plot.FarmID
	} else {
		farmID = period.FarmID
	}
	farm, err := s.farmRepo.GetByID(farmID)
	if err != nil {
		return nil, err
	}

	// 当前归属用于标注历史说法是否仍与现状一致（不改变说法本身）。
	current, cerr := s.ownershipRepo.CurrentOwnership(plot.ID)
	isCurrentOwner := cerr != nil || current.FarmID == farmID

	activities, err := s.activityRepo.ListByBatch(tc.BatchID)
	if err != nil {
		return nil, err
	}

	inspection, _ := s.inspectionRepo.GetByBatch(tc.BatchID)

	resp := &model.TraceResponse{
		Code:      code,
		FirstScan: isFirstScan,
		Batch: &model.TraceBatchInfo{
			CropID:      batch.CropID,
			SowingDate:  batch.SowingDate.Format("2006-01-02"),
			HarvestDate: "",
		},
		Farm: &model.TraceFarmInfo{
			Name:           farm.Name,
			RegionCode:     farm.RegionCode,
			PlotName:       plot.Name,
			OwnedAsOf:      anchor.Format(time.RFC3339),
			IsCurrentOwner: isCurrentOwner,
		},
		Activities: make([]model.TraceActivityInfo, 0),
	}
	if batch.HarvestDate != nil {
		resp.Batch.HarvestDate = batch.HarvestDate.Format("2006-01-02")
	}

	for _, a := range activities {
		info := model.TraceActivityInfo{
			Kind:       a.Kind,
			HappenedAt: a.HappenedAt.Format("2006-01-02"),
			Operator:   a.Operator,
			Dose:       a.Dose,
			DoseUnit:   a.DoseUnit,
		}
		resp.Activities = append(resp.Activities, info)
	}

	if inspection != nil {
		resp.Inspection = &model.TraceInspectionInfo{
			Lab:       inspection.Lab,
			SampledAt: inspection.SampledAt.Format("2006-01-02"),
			Result:    inspection.Result,
		}
	}

	return resp, nil
}

func (s *TraceCodeService) checkSafetyInterval(batch *model.CropBatch) (bool, string) {
	if batch.HarvestDate == nil {
		return true, ""
	}

	lastPesticideDate, err := s.batchRepo.GetLastPesticideDate(batch.ID)
	if err != nil || lastPesticideDate == nil {
		return true, ""
	}

	maxInterval, err := s.batchRepo.GetMaxSafeInterval(batch.ID)
	if err != nil || maxInterval == 0 {
		return true, ""
	}

	daysSincePesticide := int(batch.HarvestDate.Sub(*lastPesticideDate).Hours() / 24)
	if daysSincePesticide < maxInterval {
		return false, fmt.Sprintf("距上次施药%d天，不足安全间隔期%d天", daysSincePesticide, maxInterval)
	}
	return true, ""
}

func (s *TraceCodeService) ValidateTraceCode(code string) bool {
	return tracecode.Validate(code)
}