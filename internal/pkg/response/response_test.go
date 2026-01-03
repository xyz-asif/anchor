package response

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSuccessAndErrorResponses(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Test Success
	Success(c, map[string]string{"foo": "bar"}, "ok")
	require.Equal(t, 200, w.Code)
	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	require.Equal(t, true, body["success"])
	require.Equal(t, float64(200), body["statusCode"]) // json numbers decode to float64
	require.Equal(t, "ok", body["message"])
	require.Contains(t, body, "data")

	// Test Error
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	Error(c, 400, "bad request", "BAD_REQ")
	require.Equal(t, 400, w.Code)
	var bodyErr map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &bodyErr)
	require.NoError(t, err)
	require.Equal(t, false, bodyErr["success"])
	require.Equal(t, float64(400), bodyErr["statusCode"])
	require.Equal(t, "bad request", bodyErr["message"])
	require.Equal(t, "BAD_REQ", bodyErr["code"])
}

func TestPaginatedResponse(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	items := []map[string]any{{"id": 1}, {"id": 2}}
	Paginated(c, items, 2, 10, 1)

	require.Equal(t, 200, w.Code)
	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	require.Equal(t, true, body["success"])
	require.Equal(t, float64(200), body["statusCode"])
	data := body["data"].(map[string]any)
	require.Contains(t, data, "items")
	require.Equal(t, float64(2), data["total"].(float64))
	require.Equal(t, float64(10), data["limit"].(float64))
	require.Equal(t, float64(1), data["page"].(float64))
}
