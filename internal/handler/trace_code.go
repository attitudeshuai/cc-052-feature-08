package handler

import (
	"cc-052/internal/model"
	"cc-052/internal/service"
	"cc-052/pkg/response"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

type TraceCodeHandler struct {
	svc *service.TraceCodeService
}

func NewTraceCodeHandler(svc *service.TraceCodeService) *TraceCodeHandler {
	return &TraceCodeHandler{svc: svc}
}

func (h *TraceCodeHandler) Generate(c *gin.Context) {
	batchID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid batch id")
		return
	}

	var req model.GenerateCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	codes, err := h.svc.GenerateCodes(batchID, req.Count)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	response.Created(c, gin.H{"codes": codes, "count": len(codes)})
}

func (h *TraceCodeHandler) Trace(c *gin.Context) {
	code := c.Param("code")
	region := c.GetHeader("X-Forwarded-For")
	if region == "" {
		region = c.ClientIP()
	}

	// 可选 as_of（RFC3339）：按历史时点回放归属，只读、不改变首扫状态。
	var asOf *time.Time
	if raw := c.Query("as_of"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "invalid as_of, expect RFC3339, e.g. 2026-09-01T08:00:00Z")
			return
		}
		asOf = &t
	}

	trace, err := h.svc.Trace(code, region, asOf)
	if err != nil {
		response.NotFound(c, "trace code not found")
		return
	}
	response.Success(c, trace)
}

func (h *TraceCodeHandler) Validate(c *gin.Context) {
	code := c.Param("code")
	valid := h.svc.ValidateTraceCode(code)
	response.Success(c, gin.H{"valid": valid})
}