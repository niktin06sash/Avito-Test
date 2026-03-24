package tests

import (
	"context"
	"net/http"
	"testing"
	"time"

	"test-backend/internal/database"
	migrate "test-backend/migrations"

	"github.com/google/uuid"
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	container, dbURL, err := setupPostgres(ctx)
	if err != nil {
		panic("Failed to setup PostgreSQL container: " + err.Error())
	}
	defer container.Terminate(ctx) //nolint:errcheck
	db, err := database.New(ctx, dbURL)
	if err != nil {
		panic("Failed to connect to database: " + err.Error())
	}
	defer db.Close()
	if err := migrate.Migrate(dbURL, migrate.Migrations); err != nil {
		panic("Failed to apply migrations: " + err.Error())
	}
	globalDB = db
	m.Run()
}

// TestCompleteBookingScenario tests the main scenario:
// Create room -> Create schedule -> Create booking
func TestCompleteBookingScenario(t *testing.T) {
	setup := setupTest(t)
	client := setup.client
	baseURL := setup.baseURL
	adminToken := setup.adminToken
	userToken := setup.userToken

	roomID := createRoomAndSchedule(client, baseURL, adminToken, t)
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	slotsBefore, err := listSlots(client, baseURL, roomID, tomorrow, userToken, t)
	if err != nil {
		t.Fatalf("Failed to list slots before booking: %v", err)
	}
	initialSlotCount := len(slotsBefore)
	t.Logf("✓ Initial available slots: %d", initialSlotCount)
	slotID, err := getAvailableSlot(client, baseURL, roomID, tomorrow, userToken, t)
	if err != nil {
		t.Fatalf("Failed to get available slot: %v", err)
	}
	t.Logf("✓ Available slot found: %s", slotID)
	bookingID, err := createBooking(client, baseURL, slotID, userToken, t)
	if err != nil {
		t.Fatalf("Failed to create booking: %v", err)
	}
	t.Logf("✓ Booking created: %s", bookingID)
	slotsAfter, err := listSlots(client, baseURL, roomID, tomorrow, userToken, t)
	if err != nil {
		t.Fatalf("Failed to list slots after booking: %v", err)
	}
	afterBookingCount := len(slotsAfter)
	if afterBookingCount != initialSlotCount-1 {
		t.Fatalf("Expected slots count to decrease by 1, got %d before, %d after", initialSlotCount, afterBookingCount)
	}
	t.Logf("✓ Slots count decreased correctly: %d -> %d", initialSlotCount, afterBookingCount)

}

// TestCancellationBookingScenario tests the scenario:
// Create room -> Create schedule -> Create booking -> Cancel booking -> Cancel booking
func TestCancellationBookingScenario(t *testing.T) {
	setup := setupTest(t)
	client := setup.client
	baseURL := setup.baseURL
	adminToken := setup.adminToken
	userToken := setup.userToken

	roomID := createRoomAndSchedule(client, baseURL, adminToken, t)
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	slotsBefore, err := listSlots(client, baseURL, roomID, tomorrow, userToken, t)
	if err != nil {
		t.Fatalf("Failed to list slots before booking: %v", err)
	}
	initialSlotCount := len(slotsBefore)
	t.Logf("✓ Initial available slots: %d", initialSlotCount)
	slotID, err := getAvailableSlot(client, baseURL, roomID, tomorrow, userToken, t)
	if err != nil {
		t.Fatalf("Failed to get available slot: %v", err)
	}
	t.Logf("✓ Available slot found: %s", slotID)
	bookingID, err := createBooking(client, baseURL, slotID, userToken, t)
	if err != nil {
		t.Fatalf("Failed to create booking: %v", err)
	}
	t.Logf("✓ Booking created: %s", bookingID)
	slotsAfterBooking, err := listSlots(client, baseURL, roomID, tomorrow, userToken, t)
	if err != nil {
		t.Fatalf("Failed to list slots after booking: %v", err)
	}
	afterBookingCount := len(slotsAfterBooking)
	if afterBookingCount != initialSlotCount-1 {
		t.Fatalf("Expected slots count to decrease by 1, got %d before, %d after", initialSlotCount, afterBookingCount)
	}
	t.Logf("✓ Slots count decreased after booking: %d -> %d", initialSlotCount, afterBookingCount)
	err = cancelBooking(client, baseURL, bookingID, userToken, t)
	if err != nil {
		t.Fatalf("Failed to cancel booking: %v", err)
	}
	t.Logf("✓ Booking cancelled: %s", bookingID)
	err = cancelBooking(client, baseURL, bookingID, userToken, t)
	if err != nil {
		t.Fatalf("Failed to cancel booking: %v", err)
	}
	t.Logf("✓ Booking cancelled in second turn: %s", bookingID)
	slotsAfterCancel, err := listSlots(client, baseURL, roomID, tomorrow, userToken, t)
	if err != nil {
		t.Fatalf("Failed to list slots after cancellation: %v", err)
	}
	afterCancelCount := len(slotsAfterCancel)
	if afterCancelCount != initialSlotCount {
		t.Fatalf("Expected slots count to return to initial, got %d initial, %d after cancel", initialSlotCount, afterCancelCount)
	}

	t.Logf("✓ Slots count restored after cancellation: %d -> %d -> %d", initialSlotCount, afterBookingCount, afterCancelCount)

}

// TestDoubleBookingScenario tests the scenario:
// Create room -> Create schedule -> Create booking -> Attempt to book same slot again
func TestDoubleBookingScenario(t *testing.T) {
	setup := setupTest(t)
	client := setup.client
	baseURL := setup.baseURL
	adminToken := setup.adminToken
	userToken := setup.userToken

	roomID := createRoomAndSchedule(client, baseURL, adminToken, t)
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	slotID, err := getAvailableSlot(client, baseURL, roomID, tomorrow, userToken, t)
	if err != nil {
		t.Fatalf("Failed to get available slot: %v", err)
	}
	t.Logf("✓ Available slot found: %s", slotID)
	_, err = createBooking(client, baseURL, slotID, userToken, t)
	if err != nil {
		t.Fatalf("Failed to create first booking: %v", err)
	}
	t.Logf("✓ First booking created for slot: %s", slotID)
	status, err := createBookingWithStatus(client, baseURL, slotID, userToken, t)
	if err != nil {
		t.Fatalf("Failed to attempt second booking: %v", err)
	}
	assertStatus(http.StatusConflict, status, t)
	t.Logf("✓ Double booking correctly rejected with status %d", status)
}

// TestUnauthorizedCancelScenario tests the scenario:
// Create room -> Create schedule -> Create booking -> Attempt to cancel with different user
func TestUnauthorizedCancelScenario(t *testing.T) {
	setup := setupTest(t)
	client := setup.client
	baseURL := setup.baseURL
	adminToken := setup.adminToken
	userToken := setup.userToken

	roomID := createRoomAndSchedule(client, baseURL, adminToken, t)
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	slotID, err := getAvailableSlot(client, baseURL, roomID, tomorrow, userToken, t)
	if err != nil {
		t.Fatalf("Failed to get available slot: %v", err)
	}
	t.Logf("✓ Available slot found: %s", slotID)
	bookingID, err := createBooking(client, baseURL, slotID, userToken, t)
	if err != nil {
		t.Fatalf("Failed to create booking: %v", err)
	}
	t.Logf("✓ Booking created: %s", bookingID)
	status, err := cancelBookingWithStatus(client, baseURL, bookingID, adminToken, t)
	if err != nil {
		t.Fatalf("Failed to attempt cancel: %v", err)
	}
	assertStatus(http.StatusForbidden, status, t)
	t.Logf("✓ Unauthorized cancel correctly rejected with status %d", status)
}

// TestBookingNonExistentSlotScenario tests the scenario:
// Create booking on non-existent slot
func TestBookingNonExistentSlotScenario(t *testing.T) {
	setup := setupTest(t)
	client := setup.client
	baseURL := setup.baseURL
	userToken := setup.userToken
	nonExistentSlotID := uuid.New().String()
	t.Logf("✓ Using non-existent slot ID: %s", nonExistentSlotID)
	status, err := createBookingWithStatus(client, baseURL, nonExistentSlotID, userToken, t)
	if err != nil {
		t.Fatalf("Failed to attempt booking: %v", err)
	}
	assertStatus(http.StatusNotFound, status, t)
	t.Logf("✓ Booking non-existent slot correctly rejected with status %d", status)
}
