package router

import (
	"cc-052/internal/handler"
	"cc-052/internal/middleware"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

func Setup(
	farmH *handler.FarmHandler,
	plotH *handler.PlotHandler,
	batchH *handler.BatchHandler,
	activityH *handler.ActivityHandler,
	inspectionH *handler.InspectionHandler,
	traceCodeH *handler.TraceCodeHandler,
	ownershipH *handler.OwnershipHandler,
	healthH *handler.HealthHandler,
	rdb *redis.Client,
) *gin.Engine {
	r := gin.Default()

	// Health check
	r.GET("/healthz", healthH.Health)

	// API v1
	v1 := r.Group("/api/v1")
	{
		// Farms
		v1.POST("/farms", farmH.Create)
		v1.GET("/farms", farmH.List)
		v1.GET("/farms/:id", farmH.GetByID)

		// Plots
		v1.POST("/plots", plotH.Create)
		v1.GET("/plots/:id", plotH.GetByID)
		v1.GET("/plots", plotH.ListByFarm)

		// Plot ownership（归属变更与回放）
		v1.POST("/plots/:id/transfer", ownershipH.Transfer)
		v1.GET("/plots/:id/ownership", ownershipH.History)
		v1.GET("/plots/:id/ownership/at", ownershipH.OwnerAt)

		// Batches
		v1.POST("/batches", batchH.Create)
		v1.GET("/batches/:id", batchH.GetByID)

		// Activities
		v1.POST("/batches/:id/activities", activityH.Create)
		v1.POST("/batches/:id/activities/batch", activityH.BatchCreate)
		v1.GET("/batches/:id/activities", activityH.ListByBatch)

		// Inspections
		v1.POST("/batches/:id/inspection", inspectionH.Create)

		// Trace codes
		v1.POST("/batches/:id/codes", traceCodeH.Generate)
	}

	// Public trace endpoints with rate limiting
	traceGroup := r.Group("/api/v1/trace")
	traceGroup.Use(middleware.RateLimit(rdb, 30, time.Minute))
	{
		traceGroup.GET("/:code", traceCodeH.Trace)
		traceGroup.GET("/:code/validate", traceCodeH.Validate)
	}

	return r
}