package media

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func preview(t *testing.T, rawQuery string) (int, string) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/preview", NewHandler(nil).GetLinkPreview)
	r.POST("/upload", NewHandler(nil).UploadMedia)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/preview?"+rawQuery, nil))
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return w.Code, out["message"].(string)
}

func TestLinkPreviewMessages(t *testing.T) {
	code, msg := preview(t, "")
	require.Equal(t, 400, code)
	require.Equal(t, "Please add the link to preview, e.g. ?url=https://example.com", msg)

	code, msg = preview(t, "url=javascript:alert(1)")
	require.Equal(t, 400, code)
	require.Equal(t, "Links must start with http:// or https://", msg)

	// Internal addresses are still refused by the scraper, with a readable message
	code, msg = preview(t, "url=127.0.0.1/admin")
	require.Equal(t, 400, code)
	require.Equal(t, "We couldn't load a preview for this link. Check that the page exists and is publicly reachable", msg)
}

func TestUploadWithoutCloudinary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/upload", NewHandler(nil).UploadMedia)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/upload", nil))
	require.Equal(t, 503, w.Code)
	require.Contains(t, w.Body.String(), "File uploads aren't available right now")
}
