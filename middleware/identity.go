package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const IdentityHeader = "X-AIHub-Identity"
const ServiceHeader = "X-AIHub-Plugin-Secret"

type Identity struct {
	UserID    int    `json:"user_id"`
	PluginID  string `json:"plugin_id"`
	Method    string `json:"method"`
	URI       string `json:"uri"`
	RequestID string `json:"request_id"`
	ExpiresAt int64  `json:"expires_at"`
}

func Verify(secret, token string, request *http.Request) (Identity, bool) {
	var identity Identity
	parts := strings.Split(token, ".")
	if len(parts) != 2 || len(secret) < 32 {
		return identity, false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return identity, false
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(body, &identity) != nil {
		return identity, false
	}
	valid := identity.UserID > 0 && identity.PluginID == "example" && identity.Method == request.Method && identity.URI == request.URL.RequestURI() && identity.ExpiresAt > time.Now().Unix() && identity.ExpiresAt <= time.Now().Add(time.Minute).Unix()
	return identity, valid
}

func RequireIdentity(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := Verify(secret, c.GetHeader(IdentityHeader), c.Request)
		if !ok {
			c.AbortWithStatusJSON(401, gin.H{"success": false, "message": "宿主身份凭证无效或过期"})
			return
		}
		c.Set("user_id", identity.UserID)
		c.Next()
	}
}
func RequireService(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(secret) < 32 || !hmac.Equal([]byte(secret), []byte(c.GetHeader(ServiceHeader))) {
			c.AbortWithStatusJSON(401, gin.H{"success": false, "message": "插件服务凭据无效"})
			return
		}
		c.Next()
	}
}
