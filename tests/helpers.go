package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"test-backend/internal/database"
	"test-backend/internal/handler"
	"test-backend/internal/logger"
	"test-backend/internal/repository"
	"test-backend/internal/service"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type testSetup struct {
	client     *http.Client
	baseURL    string
	adminToken string
	userToken  string
}

var (
	globalDB *database.Database
)

func setupTest(t *testing.T) *testSetup {
	repo := repository.New(globalDB)
	serviceLog := logger.Service(logger.InitMain())
	svc := service.New(repo, serviceLog)
	handlerLog := logger.Handler(logger.InitMain())
	h := handler.New(svc, "supersecret", handlerLog)
	ts := httptest.NewServer(h.Routes())
	t.Cleanup(ts.Close)
	client := &http.Client{Timeout: 10 * time.Second}
	baseURL := ts.URL
	adminToken, err := getToken(client, baseURL, "admin", t)
	if err != nil {
		t.Fatalf("Failed to get admin token: %v", err)
	}
	userToken, err := getToken(client, baseURL, "user", t)
	if err != nil {
		t.Fatalf("Failed to get user token: %v", err)
	}
	return &testSetup{
		client:     client,
		baseURL:    baseURL,
		adminToken: adminToken,
		userToken:  userToken,
	}
}

func createRoomAndSchedule(client *http.Client, baseURL, adminToken string, t *testing.T) string {
	roomID, err := createRoom(client, baseURL, adminToken, t)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	t.Logf("✓ Room created: %s", roomID)
	err = createSchedule(client, baseURL, roomID, adminToken, t)
	if err != nil {
		t.Fatalf("Failed to create schedule: %v", err)
	}
	t.Logf("✓ Schedule created for room %s", roomID)
	return roomID
}
func setupPostgres(ctx context.Context) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "postgres",
			"POSTGRES_DB":       "booking",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to start postgres container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		return nil, "", fmt.Errorf("failed to get mapped port: %w", err)
	}

	dbURL := fmt.Sprintf("postgres://postgres:postgres@%s:%s/booking?sslmode=disable",
		host, port.Port())

	return container, dbURL, nil
}

func getToken(client *http.Client, baseURL, role string, t *testing.T) (string, error) {
	body := map[string]string{"role": role}
	bodyBytes, _ := json.Marshal(body)

	resp, err := client.Post(
		baseURL+"/dummyLogin",
		"application/json",
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get token, status: %d", resp.StatusCode)
	}

	var result map[string]string
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}
	return result["token"], nil
}

func createRoom(client *http.Client, baseURL, token string, t *testing.T) (string, error) {
	body := map[string]interface{}{
		"name":        "Meeting Room 1",
		"description": "Main conference room",
		"capacity":    10,
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", baseURL+"/rooms/create", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to create room: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create room, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", fmt.Errorf("failed to decode create room response: %w", err)
	}
	room := result["room"].(map[string]interface{})
	return room["id"].(string), nil
}

func createSchedule(client *http.Client, baseURL, roomID, token string, t *testing.T) error {
	body := map[string]interface{}{
		"daysOfWeek": []int{1, 2, 3, 4, 5},
		"startTime":  "09:00",
		"endTime":    "18:00",
	}
	bodyBytes, _ := json.Marshal(body)

	url := fmt.Sprintf("%s/rooms/%s/schedule/create", baseURL, roomID)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create schedule: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create schedule, status: %d, body: %s", resp.StatusCode, string(body))
	}
	return nil
}

func getAvailableSlot(client *http.Client, baseURL, roomID, date, token string, t *testing.T) (string, error) {
	if token == "" {
		var err error
		token, err = getToken(client, baseURL, "user", t)
		if err != nil {
			return "", fmt.Errorf("failed to get token: %w", err)
		}
	}

	slots, err := listSlots(client, baseURL, roomID, date, token, t)
	if err != nil {
		return "", fmt.Errorf("failed to list slots: %w", err)
	}
	if len(slots) == 0 {
		return "", fmt.Errorf("no available slots found")
	}

	slot := slots[0].(map[string]interface{})
	return slot["id"].(string), nil
}

func listSlots(client *http.Client, baseURL, roomID, date, token string, t *testing.T) ([]interface{}, error) {
	url := fmt.Sprintf("%s/rooms/%s/slots/list?date=%s", baseURL, roomID, date)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list slots: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list slots, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, fmt.Errorf("failed to decode list slots response: %w", err)
	}

	slots, ok := result["slots"].([]interface{})
	if !ok {
		slots = []interface{}{}
	}
	return slots, nil
}

func createBooking(client *http.Client, baseURL, slotID, token string, t *testing.T) (string, error) {
	body := map[string]interface{}{
		"slotId": slotID,
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", baseURL+"/bookings/create", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to create booking: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create booking, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", fmt.Errorf("failed to decode booking response: %w", err)
	}

	booking := result["booking"].(map[string]interface{})
	return booking["id"].(string), nil
}

func cancelBooking(client *http.Client, baseURL, bookingID, token string, t *testing.T) error {
	req, _ := http.NewRequest("POST", baseURL+"/bookings/"+bookingID+"/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to cancel booking: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to cancel booking, status: %d, body: %s", resp.StatusCode, string(body))
	}
	return nil
}

func createBookingWithStatus(client *http.Client, baseURL, slotID, token string, t *testing.T) (int, error) {
	body := map[string]interface{}{
		"slotId": slotID,
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", baseURL+"/bookings/create", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to create booking: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	return resp.StatusCode, nil
}

func cancelBookingWithStatus(client *http.Client, baseURL, bookingID, token string, t *testing.T) (int, error) {
	req, _ := http.NewRequest("POST", baseURL+"/bookings/"+bookingID+"/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to cancel booking: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	return resp.StatusCode, nil
}

func assertStatus(expected, actual int, t *testing.T) {
	if actual != expected {
		t.Fatalf("Expected status %d, got %d", expected, actual)
	}
}
