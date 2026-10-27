package router

import (
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"

	"ai-hub-example/controller"
	"ai-hub-example/hostclient"
	"ai-hub-example/middleware"
)

func New(s *controller.Service, secret string, assets fs.FS, manifest []byte) http.Handler {
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) {
		db, err := s.Store.DB.DB()
		if err != nil || db.PingContext(c.Request.Context()) != nil {
			c.Status(503)
			return
		}
		c.JSON(200, gin.H{"status": "ok"})
	})
	internal := r.Group("/internal", middleware.RequireService(secret), func(c *gin.Context) {
		if value := c.GetHeader("X-AIHub-Host-Base-URL"); value != "" {
			if err := s.Host.SetBaseURL(value); err != nil {
				c.AbortWithStatusJSON(503, gin.H{"message": "运行回调配置更新失败"})
				return
			}
		}
		if value := c.GetHeader("X-AIHub-Data-Host-Base-URL"); value != "" {
			ctx, err := hostclient.WithDataHostBaseURL(c.Request.Context(), value)
			if err != nil {
				c.AbortWithStatusJSON(400, gin.H{"message": "数据面回调地址无效"})
				return
			}
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	})
	internal.GET("/manifest", func(c *gin.Context) { c.Data(200, "application/json", manifest) })
	internal.GET("/runtime", func(c *gin.Context) {
		c.JSON(200, gin.H{"host_url": s.Host.HostBaseURL(), "data_host_url": s.Host.DataHostBaseURL()})
	})
	internal.PUT("/runtime", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		var input struct {
			HostURL     string `json:"host_url"`
			DataHostURL string `json:"data_host_url"`
		}
		if c.ShouldBindJSON(&input) != nil || s.Host.SetBaseURLs(input.HostURL, input.DataHostURL) != nil {
			c.JSON(400, gin.H{"message": "运行配置更新失败"})
			return
		}
		c.JSON(200, gin.H{"host_url": s.Host.HostBaseURL(), "data_host_url": s.Host.DataHostBaseURL()})
	})
	internal.POST("/relay/request", s.Request)
	internal.POST("/relay/response", s.Response)
	api := r.Group("/api/v1", middleware.RequireIdentity(secret), s.RequireUser)
	api.GET("/records", s.Records)
	api.GET("/records/:id", s.Record)
	r.GET("/assets/*path", func(c *gin.Context) { http.FileServer(http.FS(assets)).ServeHTTP(c.Writer, c.Request) })
	// Enable on the original writer: Gin's wrapper does not expose Unwrap, so a
	// ResponseController constructed inside the Gin handler cannot enable duplex.
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost && request.URL.Path == "/internal/relay/response" {
			if err := http.NewResponseController(w).EnableFullDuplex(); err != nil {
				http.Error(w, "full duplex required", 500)
				return
			}
		}
		r.ServeHTTP(w, request)
	})
}
