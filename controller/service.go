package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-hub-example/hostclient"
	"ai-hub-example/model"
	"ai-hub-example/processor"
)

type Service struct {
	Store *model.Store
	Host  *hostclient.Client
	Rules processor.Rules
}

func (s *Service) RequireUser(c *gin.Context) {
	user, err := s.Host.Self(c.Request.Context(), c.GetHeader(hostclient.AuthorizationHeader))
	if err != nil {
		c.AbortWithStatusJSON(503, gin.H{"success": false, "message": err.Error()})
		return
	}
	if user.ID != c.GetInt("user_id") || user.Status != 1 {
		c.AbortWithStatusJSON(403, gin.H{"success": false, "message": "用户无权访问"})
		return
	}
	c.Set("admin", user.Role >= 10)
	c.Next()
}

func (s *Service) recordQuery(c *gin.Context) *gorm.DB {
	query := s.Store.DB.WithContext(c.Request.Context()).Model(&model.Record{})
	if !c.GetBool("admin") {
		query = query.Where("user_id = ?", c.GetInt("user_id"))
	}
	return query
}

func (s *Service) Records(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "0"))
	if err != nil || page < 0 || page > 100000 {
		c.JSON(400, gin.H{"success": false, "message": "分页无效"})
		return
	}
	size, err := strconv.Atoi(c.DefaultQuery("size", "20"))
	if err != nil || size < 1 || size > 100 {
		c.JSON(400, gin.H{"success": false, "message": "分页大小无效"})
		return
	}
	query := s.recordQuery(c)
	if id := c.Query("request_id"); id != "" {
		if len(id) > 128 {
			c.JSON(400, gin.H{"success": false, "message": "请求 ID 过长"})
			return
		}
		query = query.Where("request_id = ?", id)
	}
	if point := c.Query("point"); point != "" {
		if point != "relay.request.intercept" && point != "relay.response.intercept" {
			c.JSON(400, gin.H{"success": false, "message": "扩展点无效"})
			return
		}
		query = query.Where("point = ?", point)
	}
	if status := c.Query("status"); status != "" {
		switch status {
		case "running", "completed", "error", "canceled":
		default:
			c.JSON(400, gin.H{"success": false, "message": "状态无效"})
			return
		}
		query = query.Where("status = ?", status)
	}
	for _, filter := range []struct{ name, operator string }{{"from", ">="}, {"to", "<="}} {
		if value := c.Query(filter.name); value != "" {
			timestamp, err := strconv.ParseInt(value, 10, 64)
			if err != nil || timestamp < 0 {
				c.JSON(400, gin.H{"success": false, "message": "时间筛选无效"})
				return
			}
			query = query.Where("created_at "+filter.operator+" ?", timestamp)
		}
	}
	var total int64
	if err = query.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "记录查询失败"})
		return
	}
	rows := []model.Record{}
	if err = query.Order("id DESC").Offset(page * size).Limit(size).Find(&rows).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "记录查询失败"})
		return
	}
	c.JSON(200, gin.H{"success": true, "message": "", "data": gin.H{"items": rows, "total": total, "admin": c.GetBool("admin")}})
}

func (s *Service) Record(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(400, gin.H{"success": false, "message": "记录 ID 无效"})
		return
	}
	var record model.Record
	if err = s.recordQuery(c).Where("id = ?", id).First(&record).Error; err != nil {
		status := 500
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = 404
		}
		c.JSON(status, gin.H{"success": false, "message": "记录不存在或不可访问"})
		return
	}
	var responses int64
	if err = s.recordQuery(c).Where("request_id = ? AND point = ?", record.RequestID, "relay.response.intercept").Count(&responses).Error; err != nil {
		c.JSON(500, gin.H{"success": false, "message": "关联记录查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": struct {
		model.Record
		ResponseExecuted bool `json:"response_executed"`
	}{record, responses > 0}})
}
