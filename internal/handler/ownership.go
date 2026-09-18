package handler

import (
	"cc-052/internal/model"
	"cc-052/internal/service"
	"cc-052/pkg/response"
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type OwnershipHandler struct {
	svc *service.OwnershipService
}

func NewOwnershipHandler(svc *service.OwnershipService) *OwnershipHandler {
	return &OwnershipHandler{svc: svc}
}

// Transfer POST /api/v1/plots/:id/transfer
// 转出方名下仍挂批次时返回 409 + 批次清单，带 confirm=true 重试才执行；
// from_farm_id 与当前归属不一致（含并发变更撞车）返回 409。
func (h *OwnershipHandler) Transfer(c *gin.Context) {
	plotID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid plot id")
		return
	}
	var req model.TransferPlotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	t, err := h.svc.Transfer(plotID, &req)
	if err != nil {
		var needConfirm *service.TransferNeedsConfirm
		switch {
		case errors.As(err, &needConfirm):
			response.ConflictWithData(c, needConfirm.Error(), needConfirm.Warning)
		case errors.Is(err, model.ErrPlotNotFound):
			response.NotFound(c, err.Error())
		case errors.Is(err, model.ErrFarmNotFound):
			response.BadRequest(c, "to_farm_id 对应的农场不存在")
		case errors.Is(err, model.ErrOwnerMismatch), errors.Is(err, model.ErrSameFarm):
			response.Conflict(c, err.Error())
		default:
			response.InternalError(c, err.Error())
		}
		return
	}
	response.Created(c, t)
}

// History GET /api/v1/plots/:id/ownership
// 变更前后各自挂在谁名下都查得到：归属期列表 + 每次变更动作。
func (h *OwnershipHandler) History(c *gin.Context) {
	plotID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid plot id")
		return
	}
	history, err := h.svc.History(plotID)
	if err != nil {
		if errors.Is(err, model.ErrPlotNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, history)
}

// OwnerAt GET /api/v1/plots/:id/ownership/at?time=2026-09-17（或 RFC3339），缺省为当前时刻。
func (h *OwnershipHandler) OwnerAt(c *gin.Context) {
	plotID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid plot id")
		return
	}
	at := time.Now()
	if s := c.Query("time"); s != "" {
		t, err := parseTimeParam(s)
		if err != nil {
			response.BadRequest(c, "invalid time, use RFC3339 or YYYY-MM-DD")
			return
		}
		at = t
	}
	owner, err := h.svc.OwnerAt(plotID, at)
	if err != nil {
		if errors.Is(err, model.ErrPlotNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, owner)
}

func parseTimeParam(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", s)
}
