package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/batyray/notification-system/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMetrics_Success(t *testing.T) {
	h, _ := setupTestHandler(t)

	h.DB.Create(&models.Notification{Recipient: "a", Channel: models.ChannelSMS, Content: "m", Status: models.StatusSent, CorrelationID: "c1"})
	h.DB.Create(&models.Notification{Recipient: "b", Channel: models.ChannelSMS, Content: "m", Status: models.StatusSent, CorrelationID: "c2"})
	h.DB.Create(&models.Notification{Recipient: "c", Channel: models.ChannelSMS, Content: "m", Status: models.StatusFailed, CorrelationID: "c3"})
	h.DB.Create(&models.Notification{Recipient: "d", Channel: models.ChannelEmail, Content: "m", Status: models.StatusPending, CorrelationID: "c4"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	w := httptest.NewRecorder()
	h.GetMetrics(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	delivery := resp["delivery"].(map[string]interface{})
	assert.Equal(t, float64(2), delivery["total_sent"])
	assert.Equal(t, float64(1), delivery["total_failed"])
}
