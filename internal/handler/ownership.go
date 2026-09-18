package handler

import (
	"cc-052/internal/model"
	"cc-052/internal/repository"
	"cc-052/internal/service"
	"cc-052/pkg/response"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
)

type OwnershipHandler struct {
	svc *service.OwnershipService
}

func NewOwnershipHandler(svc *service.OwnershipService) *OwnershipHandler {
	return &OwnershipHandler{svc: svc}
}

// Precheck 变更前预检：GET /plots/:id/ownership/precheck?to_farm_id=
func (h *OwnershipHandler) Precheck(c *gin.Context) {
	plotID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid plot id")
		return
	}
	toFarmID, err := strconv.ParseInt(c.Query("to_farm_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid to_farm_id")
		return
	}
	pc, err := h.svc.Precheck(plotID, toFarmID)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, pc)
}

// Transfer 执行归属变更：POST /plots/:id/ownership/transfers
func (h *OwnershipHandler) Transfer(c *gin.Context) {
	plotID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid plot id")
		return
	}
	var req model.TransferOwnershipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	t, err := h.svc.Transfer(plotID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSameFarm):
			response.BadRequest(c, err.Error())
		case errors.Is(err, service.ErrHangingBatches):
			// 409：名下仍挂批次，需带 confirm_hanging_batches 确认后重试
			response.Error(c, 409, err.Error())
		case errors.Is(err, service.ErrOwnershipConflict):
			response.Error(c, 409, err.Error())
		case errors.Is(err, repository.ErrNoCurrentOwnership):
			response.Error(c, 409, err.Error())
		default:
			response.InternalError(c, err.Error())
		}
		return
	}
	response.Created(c, t)
}

// ListTransfers 回看变更动作：GET /plots/:id/ownership/transfers
func (h *OwnershipHandler) ListTransfers(c *gin.Context) {
	plotID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid plot id")
		return
	}
	ts, err := h.svc.ListTransfers(plotID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, ts)
}

// ListPeriods 查看归属时间线：GET /plots/:id/ownership/periods
func (h *OwnershipHandler) ListPeriods(c *gin.Context) {
	plotID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid plot id")
		return
	}
	ps, err := h.svc.ListPeriods(plotID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, ps)
}
