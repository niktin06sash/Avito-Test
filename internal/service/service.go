package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"

	"test-backend/internal/apperrors"
	"test-backend/internal/model"
)

const slotDuration = 30 * time.Minute

var (
	fixedAdminID = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	fixedUserID  = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

type Repository interface {
	CreateUser(ctx context.Context, user model.User) (model.User, error)
	EnsureUser(ctx context.Context, user model.User) error
	GetUserByEmail(ctx context.Context, email string) (model.User, error)
	CreateRoom(ctx context.Context, room model.Room) (model.Room, error)
	ListRooms(ctx context.Context) ([]model.Room, error)
	RoomExists(ctx context.Context, roomID uuid.UUID) (bool, error)
	CreateSchedule(ctx context.Context, schedule model.Schedule) (model.Schedule, error)
	SaveSlots(ctx context.Context, slots []model.Slot) error
	ListAvailableSlotsByRoomAndDate(ctx context.Context, roomID uuid.UUID, date time.Time) ([]model.Slot, error)
	GetSlotByID(ctx context.Context, slotID uuid.UUID) (model.Slot, error)
	CreateBooking(ctx context.Context, b model.Booking) (model.Booking, error)
	ListBookings(ctx context.Context, page, pageSize int) ([]model.Booking, int, error)
	ListMyFutureBookings(ctx context.Context, userID uuid.UUID, now time.Time) ([]model.Booking, error)
	GetBooking(ctx context.Context, bookingID uuid.UUID) (model.Booking, error)
	CancelBooking(ctx context.Context, bookingID uuid.UUID) (model.Booking, error)
}

type Service struct {
	repo Repository
	log  *logrus.Entry
}

func New(repo Repository, log *logrus.Entry) *Service {
	return &Service{repo: repo, log: log}
}

func (s *Service) EnsureSystemUsers(ctx context.Context) error {
	now := time.Now().UTC()
	if err := s.repo.EnsureUser(ctx, model.User{
		ID:           fixedAdminID,
		Email:        "admin@dummy.local",
		PasswordHash: "dummy",
		Role:         model.RoleAdmin,
		CreatedAt:    now,
	}); err != nil {
		return err
	}
	return s.repo.EnsureUser(ctx, model.User{
		ID:           fixedUserID,
		Email:        "user@dummy.local",
		PasswordHash: "dummy",
		Role:         model.RoleUser,
		CreatedAt:    now,
	})
}

func (s *Service) DummyLogin(ctx context.Context, role model.Role) (uuid.UUID, model.Role, error) {
	_ = ctx
	var id uuid.UUID
	switch role {
	case model.RoleAdmin:
		id = fixedAdminID
	case model.RoleUser:
		id = fixedUserID
	default:
		s.log.WithField("role", role).Warn("invalid role for dummy login")
		return uuid.Nil, "", apperrors.BadRequest
	}
	s.log.WithFields(logrus.Fields{"user_id": id, "role": role}).Info("dummy login")
	return id, role, nil
}

func (s *Service) Register(ctx context.Context, email, password string, role model.Role) (model.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return model.User{}, err
	}
	user := model.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: string(hash),
		Role:         role,
		CreatedAt:    time.Now().UTC(),
	}
	return s.repo.CreateUser(ctx, user)
}

func (s *Service) Login(ctx context.Context, email, password string) (uuid.UUID, model.Role, error) {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return uuid.Nil, "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return uuid.Nil, "", apperrors.Forbidden
	}
	return user.ID, user.Role, nil
}

func (s *Service) CreateRoom(ctx context.Context, name string, description *string, capacity *int) (model.Room, error) {
	return s.repo.CreateRoom(ctx, model.Room{
		ID:          uuid.New(),
		Name:        name,
		Description: description,
		Capacity:    capacity,
		CreatedAt:   time.Now().UTC(),
	})
}

func (s *Service) ListRooms(ctx context.Context) ([]model.Room, error) { return s.repo.ListRooms(ctx) }

func (s *Service) CreateSchedule(ctx context.Context, roomID uuid.UUID, days []int, startTime, endTime string) (model.Schedule, error) {
	ok, err := s.repo.RoomExists(ctx, roomID)
	if err != nil {
		return model.Schedule{}, err
	}
	if !ok {
		return model.Schedule{}, apperrors.NotFound
	}

	startClock, endClock, err := parseClockRange(startTime, endTime)
	if err != nil {
		return model.Schedule{}, apperrors.BadRequest
	}
	for _, d := range days {
		if d < 1 || d > 7 {
			return model.Schedule{}, apperrors.BadRequest
		}
	}

	schedule, err := s.repo.CreateSchedule(ctx, model.Schedule{
		ID:         uuid.New(),
		RoomID:     roomID,
		DaysOfWeek: days,
		StartTime:  startTime,
		EndTime:    endTime,
	})
	if err != nil {
		return model.Schedule{}, err
	}

	slots := make([]model.Slot, 0, 400)
	now := time.Now().UTC()
	baseDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for i := 0; i < 30; i++ {
		day := baseDate.AddDate(0, 0, i)
		wd := int(day.Weekday())
		if wd == 0 {
			wd = 7
		}
		if !containsDay(days, wd) {
			continue
		}
		cur := time.Date(day.Year(), day.Month(), day.Day(), startClock.Hour(), startClock.Minute(), 0, 0, time.UTC)
		limit := time.Date(day.Year(), day.Month(), day.Day(), endClock.Hour(), endClock.Minute(), 0, 0, time.UTC)
		for cur.Before(limit) {
			slotEnd := cur.Add(slotDuration)
			if slotEnd.After(limit) {
				break
			}
			slots = append(slots, model.Slot{
				ID:     uuid.New(),
				RoomID: roomID,
				Start:  cur,
				End:    slotEnd,
			})
			cur = slotEnd
		}
	}
	if err = s.repo.SaveSlots(ctx, slots); err != nil {
		return model.Schedule{}, err
	}
	return schedule, nil
}

func (s *Service) ListAvailableSlots(ctx context.Context, roomID uuid.UUID, date time.Time) ([]model.Slot, error) {
	ok, err := s.repo.RoomExists(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperrors.NotFound
	}
	return s.repo.ListAvailableSlotsByRoomAndDate(ctx, roomID, date)
}

func (s *Service) CreateBooking(ctx context.Context, slotID, userID uuid.UUID, createConferenceLink bool) (model.Booking, error) {
	slot, err := s.repo.GetSlotByID(ctx, slotID)
	if err != nil {
		return model.Booking{}, err
	}
	if slot.Start.Before(time.Now().UTC()) {
		return model.Booking{}, apperrors.BadRequest
	}
	var link *string
	if createConferenceLink {
		v := fmt.Sprintf("https://meet.example.local/%s", uuid.NewString())
		link = &v
	}
	return s.repo.CreateBooking(ctx, model.Booking{
		ID:             uuid.New(),
		SlotID:         slotID,
		UserID:         userID,
		Status:         model.BookingActive,
		ConferenceLink: link,
		CreatedAt:      time.Now().UTC(),
	})
}

func (s *Service) ListBookings(ctx context.Context, page, pageSize int) ([]model.Booking, int, error) {
	if page < 1 || pageSize < 1 || pageSize > 100 {
		return nil, 0, apperrors.BadRequest
	}
	return s.repo.ListBookings(ctx, page, pageSize)
}

func (s *Service) ListMyBookings(ctx context.Context, userID uuid.UUID) ([]model.Booking, error) {
	return s.repo.ListMyFutureBookings(ctx, userID, time.Now().UTC())
}

func (s *Service) CancelBooking(ctx context.Context, bookingID, userID uuid.UUID) (model.Booking, error) {
	booking, err := s.repo.GetBooking(ctx, bookingID)
	if err != nil {
		return model.Booking{}, err
	}
	if booking.UserID != userID {
		return model.Booking{}, apperrors.Forbidden
	}
	if booking.Status == model.BookingCancelled {
		return booking, nil
	}
	return s.repo.CancelBooking(ctx, bookingID)
}
