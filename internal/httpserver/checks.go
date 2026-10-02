package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"endpoint-monitor/internal/check"
	"endpoint-monitor/internal/checker"
	"endpoint-monitor/internal/monitor"
	"endpoint-monitor/internal/probe"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultProbeTimeout = 10 * time.Second

type Server struct {
	pool   *pgxpool.Pool
	prober *probe.Prober
}

func NewRouter(pool *pgxpool.Pool, prober *probe.Prober) (*gin.Engine, error) {
	return newRouter(pool, prober)
}

func newRouter(pool *pgxpool.Pool, prober *probe.Prober) (*gin.Engine, error) {
	if prober == nil {
		prober = probe.NewProber(defaultProbeTimeout)
	}

	router := gin.New()
	router.Use(gin.Recovery())

	if err := router.SetTrustedProxies(nil); err != nil {
		return nil, err
	}

	s := &Server{pool: pool, prober: prober}

	router.GET("/health", healthHandler)
	router.POST("/api/monitors", s.createMonitor)
	router.GET("/api/monitors/:id", s.getMonitor)
	router.GET("/api/monitors", s.listMonitors)
	router.POST("/api/monitors/:id/check", s.checkMonitor)
	router.GET("/api/monitors/:id/checks", s.listMonitorChecks)

	return router, nil
}

func (s *Server) checkMonitor(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	ctx := c.Request.Context()

	m, err := monitor.GetByID(ctx, s.pool, id)
	if errors.Is(err, monitor.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "monitor not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get monitor"})
		return
	}

	created, err := checker.Run(ctx, s.pool, s.prober, m)
	if err != nil {
		if errors.Is(err, probe.ErrBlockedHost) ||
			errors.Is(err, probe.ErrInvalidScheme) ||
			errors.Is(err, probe.ErrInvalidURL) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to run check"})
		return
	}

	c.JSON(http.StatusCreated, created)
}

func (s *Server) listMonitorChecks(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	ctx := c.Request.Context()

	_, err = monitor.GetByID(ctx, s.pool, id)
	if errors.Is(err, monitor.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "monitor not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get monitor"})
		return
	}

	checks, err := check.ListByMonitorID(ctx, s.pool, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list checks"})
		return
	}

	c.JSON(http.StatusOK, checks)
}
