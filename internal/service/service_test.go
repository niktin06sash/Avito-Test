package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"

	"test-backend/internal/apperrors"
	"test-backend/internal/model"
	mock_service "test-backend/internal/service/mocks"
)

func setupService(t *testing.T) (*Service, *mock_service.MockRepository, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mock_service.NewMockRepository(ctrl)
	log := logrus.NewEntry(logrus.New())
	return New(repo, log), repo, ctrl
}

func TestRegister(t *testing.T) {
	cases := []struct {
		name      string
		email     string
		password  string
		role      model.Role
		repoErr   error
		wantError bool
	}{
		{name: "success", email: "alice@test.local", password: "pw", role: model.RoleUser, repoErr: nil, wantError: false},
		{name: "create user error", email: "bob@test.local", password: "pw", role: model.RoleUser, repoErr: apperrors.ErrFooConflict, wantError: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, repo, ctrl := setupService(t)
			defer ctrl.Finish()

			repo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, u model.User) (model.User, error) {
				if tc.repoErr != nil {
					return model.User{}, tc.repoErr
				}
				return u, nil
			})

			user, err := service.Register(context.Background(), tc.email, tc.password, tc.role)
			if tc.wantError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if user.Email != tc.email || user.Role != tc.role {
				t.Fatalf("unexpected user registered: %#v", user)
			}
		})
	}
}

func TestLogin(t *testing.T) {
	passHash, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	cases := []struct {
		name      string
		email     string
		password  string
		user      model.User
		repoErr   error
		wantError error
	}{
		{name: "ok", email: "a@test.local", password: "secret", user: model.User{ID: uuid.New(), Email: "a@test.local", PasswordHash: string(passHash), Role: model.RoleUser}, repoErr: nil, wantError: nil},
		{name: "wrong password", email: "a@test.local", password: "wrong", user: model.User{ID: uuid.New(), Email: "a@test.local", PasswordHash: string(passHash), Role: model.RoleUser}, repoErr: nil, wantError: apperrors.ErrFooForbidden},
		{name: "not found", email: "b@test.local", password: "secret", user: model.User{}, repoErr: apperrors.ErrFooNotFound, wantError: apperrors.ErrFooNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, repo, ctrl := setupService(t)
			defer ctrl.Finish()

			repo.EXPECT().GetUserByEmail(gomock.Any(), tc.email).Return(tc.user, tc.repoErr)

			uid, role, err := service.Login(context.Background(), tc.email, tc.password)
			if tc.wantError != nil {
				if !errors.Is(err, tc.wantError) {
					t.Fatalf("expected %v got %v", tc.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if uid != tc.user.ID || role != tc.user.Role {
				t.Fatalf("unexpected login result: %v %v", uid, role)
			}
		})
	}
}

func TestCreateRoom(t *testing.T) {
	cases := []struct {
		name      string
		roomName  string
		desc      *string
		capacity  *int
		repoErr   error
		wantError bool
	}{
		{name: "ok", roomName: "room1", desc: nil, capacity: nil, repoErr: nil, wantError: false},
		{name: "repo error", roomName: "room2", desc: nil, capacity: nil, repoErr: apperrors.ErrFooConflict, wantError: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, repo, ctrl := setupService(t)
			defer ctrl.Finish()

			repo.EXPECT().CreateRoom(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, r model.Room) (model.Room, error) {
				if tc.repoErr != nil {
					return model.Room{}, tc.repoErr
				}
				return r, nil
			})

			room, err := service.CreateRoom(context.Background(), tc.roomName, tc.desc, tc.capacity)
			if tc.wantError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if room.Name != tc.roomName {
				t.Fatalf("unexpected room name %q", room.Name)
			}
		})
	}
}

func TestCreateSchedule(t *testing.T) {
	okRoomID := uuid.New()
	cases := []struct {
		name         string
		roomExists   bool
		existsErr    error
		days         []int
		start        string
		end          string
		createErr    error
		saveErr      error
		wantErr      error
		expectCreate bool
		expectSave   bool
	}{
		{name: "repo error on exists", roomExists: false, existsErr: apperrors.ErrFooConflict, wantErr: apperrors.ErrFooConflict},
		{name: "room not found", roomExists: false, existsErr: nil, days: []int{1}, start: "09:00", end: "10:00", wantErr: apperrors.ErrFooNotFound},
		{name: "invalid days", roomExists: true, existsErr: nil, days: []int{0}, start: "09:00", end: "10:00", wantErr: apperrors.ErrFooBadRequest},
		{name: "invalid start time", roomExists: true, existsErr: nil, days: []int{1}, start: "bad", end: "10:00", wantErr: apperrors.ErrFooBadRequest},
		{name: "invalid end time", roomExists: true, existsErr: nil, days: []int{1}, start: "09:00", end: "bad", wantErr: apperrors.ErrFooBadRequest},
		{name: "start time less than end time", roomExists: true, existsErr: nil, days: []int{1}, start: "10:00", end: "09:00", wantErr: apperrors.ErrFooBadRequest},
		{name: "create schedule error", roomExists: true, existsErr: nil, days: []int{1}, start: "09:00", end: "10:00", createErr: apperrors.ErrFooConflict, wantErr: apperrors.ErrFooConflict, expectCreate: true},
		{name: "save slots error", roomExists: true, existsErr: nil, days: []int{1, 2, 3, 4, 5, 6, 7}, start: "09:00", end: "10:00", saveErr: apperrors.ErrFooConflict, wantErr: apperrors.ErrFooConflict, expectCreate: true, expectSave: true},
		{name: "success", roomExists: true, existsErr: nil, days: []int{1, 2, 3, 4, 5, 7}, start: "09:00", end: "10:10", wantErr: nil, expectCreate: true, expectSave: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, repo, ctrl := setupService(t)
			defer ctrl.Finish()

			repo.EXPECT().RoomExists(gomock.Any(), okRoomID).Return(tc.roomExists, tc.existsErr)
			if !tc.roomExists || tc.existsErr != nil {
				_, err := service.CreateSchedule(context.Background(), okRoomID, tc.days, tc.start, tc.end)
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v, got %v", tc.wantErr, err)
				}
				return
			}

			if !tc.expectCreate {
				_, err := service.CreateSchedule(context.Background(), okRoomID, tc.days, tc.start, tc.end)
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v got %v", tc.wantErr, err)
				}
				return
			}

			repo.EXPECT().CreateSchedule(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, s model.Schedule) (model.Schedule, error) {
				if tc.createErr != nil {
					return model.Schedule{}, tc.createErr
				}
				return s, nil
			})

			if tc.expectSave {
				repo.EXPECT().SaveSlots(gomock.Any(), gomock.Any()).Return(tc.saveErr)
			}

			out, err := service.CreateSchedule(context.Background(), okRoomID, tc.days, tc.start, tc.end)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.RoomID != okRoomID {
				t.Fatalf("unexpected room in schedule: %v", out.RoomID)
			}
		})
	}
}

func TestListAvailableSlots(t *testing.T) {
	okRoomID := uuid.New()
	cases := []struct {
		name       string
		roomExists bool
		existsErr  error
		slots      []model.Slot
		repoErr    error
		wantErr    error
	}{
		{name: "room not found", roomExists: false, existsErr: nil, wantErr: apperrors.ErrFooNotFound},
		{name: "repo error on exists", roomExists: false, existsErr: apperrors.ErrFooConflict, wantErr: apperrors.ErrFooConflict},
		{name: "list slots success", roomExists: true, existsErr: nil, slots: []model.Slot{{ID: uuid.New()}}, repoErr: nil, wantErr: nil},
		{name: "list slots fail", roomExists: true, existsErr: nil, slots: nil, repoErr: apperrors.ErrFooConflict, wantErr: apperrors.ErrFooConflict},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, repo, ctrl := setupService(t)
			defer ctrl.Finish()

			repo.EXPECT().RoomExists(gomock.Any(), okRoomID).Return(tc.roomExists, tc.existsErr)
			if tc.wantErr != nil && !tc.roomExists {
				_, err := service.ListAvailableSlots(context.Background(), okRoomID, time.Now())
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v got %v", tc.wantErr, err)
				}
				return
			}

			if tc.roomExists {
				repo.EXPECT().ListAvailableSlotsByRoomAndDate(gomock.Any(), okRoomID, gomock.Any()).Return(tc.slots, tc.repoErr)
			}

			slots, err := service.ListAvailableSlots(context.Background(), okRoomID, time.Now())
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(slots) != len(tc.slots) {
				t.Fatalf("expected %d slots got %d", len(tc.slots), len(slots))
			}
		})
	}
}

func TestCreateBooking(t *testing.T) {
	slotID := uuid.New()
	userID := uuid.New()
	futureSlot := model.Slot{ID: slotID, Start: time.Now().UTC().Add(1 * time.Hour), End: time.Now().UTC().Add(2 * time.Hour)}
	pastSlot := model.Slot{ID: slotID, Start: time.Now().UTC().Add(-2 * time.Hour), End: time.Now().UTC().Add(-1 * time.Hour)}
	cases := []struct {
		name                 string
		slot                 model.Slot
		createConferenceLink bool
		getSlotErr           error
		createBookingErr     error
		wantErr              error
		wantLink             bool
	}{
		{name: "slot not found", slot: model.Slot{}, getSlotErr: apperrors.ErrFooNotFound, wantErr: apperrors.ErrFooNotFound},
		{name: "slot in past", slot: pastSlot, getSlotErr: nil, wantErr: apperrors.ErrFooBadRequest},
		{name: "repo create booking error", slot: futureSlot, getSlotErr: nil, createBookingErr: apperrors.ErrFooConflict, wantErr: apperrors.ErrFooConflict},
		{name: "success no link", slot: futureSlot, getSlotErr: nil, createConferenceLink: false, wantErr: nil, wantLink: false},
		{name: "success with link", slot: futureSlot, getSlotErr: nil, createConferenceLink: true, wantErr: nil, wantLink: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, repo, ctrl := setupService(t)
			defer ctrl.Finish()

			repo.EXPECT().GetSlotByID(gomock.Any(), slotID).Return(tc.slot, tc.getSlotErr)
			if tc.getSlotErr != nil || tc.wantErr == apperrors.ErrFooBadRequest {
				_, err := service.CreateBooking(context.Background(), slotID, userID, tc.createConferenceLink)
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v got %v", tc.wantErr, err)
				}
				return
			}

			repo.EXPECT().CreateBooking(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, b model.Booking) (model.Booking, error) {
				if tc.createBookingErr != nil {
					return model.Booking{}, tc.createBookingErr
				}
				return b, nil
			})

			out, err := service.CreateBooking(context.Background(), slotID, userID, tc.createConferenceLink)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.UserID != userID || out.SlotID != slotID {
				t.Fatalf("unexpected booking: %#v", out)
			}
			if tc.wantLink && out.ConferenceLink == nil {
				t.Fatal("expected conference link")
			}
			if !tc.wantLink && out.ConferenceLink != nil {
				t.Fatal("did not expect conference link")
			}
		})
	}
}

func TestCancelBooking(t *testing.T) {
	bookingID := uuid.New()
	ownerID := uuid.New()
	otherID := uuid.New()
	cases := []struct {
		name       string
		booking    model.Booking
		getErr     error
		cancelErr  error
		userID     uuid.UUID
		wantErr    error
		wantStatus model.BookingStatus
	}{
		{name: "get error", getErr: apperrors.ErrFooNotFound, userID: ownerID, wantErr: apperrors.ErrFooNotFound},
		{name: "forbidden", booking: model.Booking{ID: bookingID, UserID: ownerID, Status: model.BookingActive}, userID: otherID, wantErr: apperrors.ErrFooForbidden},
		{name: "already cancelled", booking: model.Booking{ID: bookingID, UserID: ownerID, Status: model.BookingCancelled}, userID: ownerID, wantErr: nil, wantStatus: model.BookingCancelled},
		{name: "success", booking: model.Booking{ID: bookingID, UserID: ownerID, Status: model.BookingActive}, userID: ownerID, cancelErr: nil, wantErr: nil, wantStatus: model.BookingCancelled},
		{name: "cancel repo error", booking: model.Booking{ID: bookingID, UserID: ownerID, Status: model.BookingActive}, userID: ownerID, cancelErr: apperrors.ErrFooConflict, wantErr: apperrors.ErrFooConflict},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, repo, ctrl := setupService(t)
			defer ctrl.Finish()

			repo.EXPECT().GetBooking(gomock.Any(), bookingID).Return(tc.booking, tc.getErr)
			if tc.getErr != nil || tc.wantErr == apperrors.ErrFooForbidden || tc.booking.Status == model.BookingCancelled {
				out, err := service.CancelBooking(context.Background(), bookingID, tc.userID)
				if tc.wantErr != nil {
					if !errors.Is(err, tc.wantErr) {
						t.Fatalf("expected %v got %v", tc.wantErr, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if out.Status != tc.wantStatus {
					t.Fatalf("expected status %v got %v", tc.wantStatus, out.Status)
				}
				return
			}

			repo.EXPECT().CancelBooking(gomock.Any(), bookingID).Return(tc.cancelErr)
			out, err := service.CancelBooking(context.Background(), bookingID, tc.userID)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Status != tc.wantStatus {
				t.Fatalf("expected status %v got %v", tc.wantStatus, out.Status)
			}
		})
	}
}

func TestListBookings(t *testing.T) {
	cases := []struct {
		name      string
		page      int
		pageSize  int
		repoRes   []model.Booking
		repoTotal int
		repoErr   error
		wantErr   error
	}{
		{name: "invalid paging", page: 0, pageSize: 10, wantErr: apperrors.ErrFooBadRequest},
		{name: "repo error", page: 1, pageSize: 10, repoErr: apperrors.ErrFooConflict, wantErr: apperrors.ErrFooConflict},
		{name: "success", page: 2, pageSize: 10, repoRes: []model.Booking{{ID: uuid.New()}}, repoTotal: 1, wantErr: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, repo, ctrl := setupService(t)
			defer ctrl.Finish()

			if tc.wantErr == apperrors.ErrFooBadRequest {
				_, _, err := service.ListBookings(context.Background(), tc.page, tc.pageSize)
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v got %v", tc.wantErr, err)
				}
				return
			}

			repo.EXPECT().ListBookings(gomock.Any(), tc.page, tc.pageSize).Return(tc.repoRes, tc.repoTotal, tc.repoErr)
			out, total, err := service.ListBookings(context.Background(), tc.page, tc.pageSize)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if total != tc.repoTotal || len(out) != len(tc.repoRes) {
				t.Fatalf("unexpected result, total=%d len=%d", total, len(out))
			}
		})
	}
}

func TestListMyBookings(t *testing.T) {
	userID := uuid.New()
	cases := []struct {
		name    string
		userID  uuid.UUID
		repoRes []model.Booking
		repoErr error
		wantErr error
	}{
		{name: "repo error", userID: userID, repoErr: apperrors.ErrFooConflict, wantErr: apperrors.ErrFooConflict},
		{name: "success", userID: userID, repoRes: []model.Booking{{ID: uuid.New(), UserID: userID}}, wantErr: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, repo, ctrl := setupService(t)
			defer ctrl.Finish()

			repo.EXPECT().ListMyFutureBookings(gomock.Any(), tc.userID, gomock.Any()).Return(tc.repoRes, tc.repoErr)
			out, err := service.ListMyBookings(context.Background(), tc.userID)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(out) != len(tc.repoRes) {
				t.Fatalf("expected %d got %d", len(tc.repoRes), len(out))
			}
		})
	}
}

func TestListRooms(t *testing.T) {
	cases := []struct {
		name    string
		rooms   []model.Room
		repoErr error
		wantErr error
	}{
		{name: "success", rooms: []model.Room{{ID: uuid.New(), Name: "A"}}, repoErr: nil, wantErr: nil},
		{name: "repo error", rooms: nil, repoErr: apperrors.ErrFooConflict, wantErr: apperrors.ErrFooConflict},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, repo, ctrl := setupService(t)
			defer ctrl.Finish()

			repo.EXPECT().ListRooms(gomock.Any()).Return(tc.rooms, tc.repoErr)
			out, err := service.ListRooms(context.Background())
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(out) != len(tc.rooms) {
				t.Fatalf("expected %d got %d", len(tc.rooms), len(out))
			}
		})
	}
}

func TestDummyLogin(t *testing.T) {
	cases := []struct {
		expectCalled bool
		name         string
		role         model.Role
		wantID       uuid.UUID
		wantErr      error
	}{
		{name: "admin success", role: model.RoleAdmin, wantID: fixedAdminID, wantErr: nil, expectCalled: true},
		{name: "user success", role: model.RoleUser, wantID: fixedUserID, wantErr: nil, expectCalled: true},
		{name: "invalid role", role: "invalid", wantID: uuid.Nil, wantErr: apperrors.ErrFooBadRequest, expectCalled: false},
		{name: "admin dummy fail", role: model.RoleAdmin, wantErr: apperrors.ErrFooConflict, wantID: uuid.Nil, expectCalled: true},
		{name: "user ensure fail", role: model.RoleUser, wantID: uuid.Nil, wantErr: apperrors.ErrFooConflict, expectCalled: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, repo, ctrl := setupService(t)
			defer ctrl.Finish()
			if tc.expectCalled {
				expected := repo.EXPECT().EnsureUser(gomock.Any(), gomock.Any())
				if tc.wantErr != nil {
					expected.Return(tc.wantErr)
				} else {
					expected.Return(nil)
				}
			}
			id, role, err := service.DummyLogin(context.Background(), tc.role)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tc.wantID || role != tc.role {
				t.Fatalf("unexpected login result: %v %v", id, role)
			}
		})
	}
}
