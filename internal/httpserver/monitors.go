package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"endpoint-monitor/internal/monitor"

	"github.com/gin-gonic/gin"
)

type createMonitorRequest struct {
	Name            string `json:"name"`
	URL             string `json:"url"`
	IntervalSeconds int    `json:"interval_seconds"`
}

func (s *Server) createMonitor(c *gin.Context) {
	var req createMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if req.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}
	if req.IntervalSeconds <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "interval_seconds must be greater than 0"})
		return
	}

	created, err := monitor.Create(c.Request.Context(), s.pool, req.Name, req.URL, req.IntervalSeconds)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create monitor"})
		return
	}

	c.JSON(http.StatusCreated, created)
}

func (s *Server) getMonitor(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	m, err := monitor.GetByID(c.Request.Context(), s.pool, id)
	if errors.Is(err, monitor.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "monitor not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get monitor"})
		return
	}

	c.JSON(http.StatusOK, m)
}

func (s *Server) listMonitors(c *gin.Context) {
	monitors, err := monitor.List(c.Request.Context(), s.pool)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list monitors"})
		return
	}

	c.JSON(http.StatusOK, monitors)
}
