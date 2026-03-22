package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"test-backend/internal/apperrors"
	"test-backend/internal/database"
	"test-backend/internal/model"
)

type Repository struct {
	db *database.Database
}

func New(db *database.Database) *Repository {
	return &Repository{db: db}
}

func (p *Repository) CreateUser(ctx context.Context, user model.User) (model.User, error) {
	const q = `insert into users(id,email,password_hash,role,created_at) values($1,$2,$3,$4,$5)`
	_, err := p.db.Pool().Exec(ctx, q, user.ID, user.Email, user.PasswordHash, user.Role, user.CreatedAt)
	if err != nil {
		if uniqueError(err) {
			return model.User{}, apperrors.Conflict
		}
		return model.User{}, err
	}
	return user, nil
}

func (p *Repository) EnsureUser(ctx context.Context, user model.User) error {
	const q = `insert into users(id,email,password_hash,role,created_at) values($1,$2,$3,$4,$5) on conflict (id) do nothing`
	_, err := p.db.Pool().Exec(ctx, q, user.ID, user.Email, user.PasswordHash, user.Role, user.CreatedAt)
	return err
}

func (p *Repository) GetUserByEmail(ctx context.Context, email string) (model.User, error) {
	const q = `select id,email,password_hash,role,created_at from users where email=$1`
	var u model.User
	err := p.db.Pool().QueryRow(ctx, q, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, apperrors.NotFound
	}
	return u, err
}

func (p *Repository) CreateRoom(ctx context.Context, room model.Room) (model.Room, error) {
	const q = `insert into rooms(id,name,description,capacity,created_at) values($1,$2,$3,$4,$5)`
	_, err := p.db.Pool().Exec(ctx, q, room.ID, room.Name, room.Description, room.Capacity, room.CreatedAt)
	return room, err
}

func (p *Repository) ListRooms(ctx context.Context) ([]model.Room, error) {
	rows, err := p.db.Pool().Query(ctx, `select id,name,description,capacity,created_at from rooms order by created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rooms []model.Room
	for rows.Next() {
		var r model.Room
		if err = rows.Scan(&r.ID, &r.Name, &r.Description, &r.Capacity, &r.CreatedAt); err != nil {
			return nil, err
		}
		rooms = append(rooms, r)
	}
	return rooms, rows.Err()
}

func (p *Repository) RoomExists(ctx context.Context, roomID uuid.UUID) (bool, error) {
	var ok bool
	err := p.db.Pool().QueryRow(ctx, `select exists(select 1 from rooms where id=$1)`, roomID).Scan(&ok)
	return ok, err
}

func (p *Repository) CreateSchedule(ctx context.Context, schedule model.Schedule) (model.Schedule, error) {
	daysJSON, _ := json.Marshal(schedule.DaysOfWeek)
	const q = `insert into schedules(id,room_id,days_of_week,start_time,end_time,created_at) values($1,$2,$3,$4,$5,$6)`
	_, err := p.db.Pool().Exec(ctx, q, schedule.ID, schedule.RoomID, daysJSON, schedule.StartTime, schedule.EndTime, time.Now().UTC())
	if err != nil {
		if uniqueError(err) {
			return model.Schedule{}, apperrors.Conflict
		}
		return model.Schedule{}, err
	}
	return schedule, nil
}

func (p *Repository) SaveSlots(ctx context.Context, slots []model.Slot) error {
	batch := &pgx.Batch{}
	for _, s := range slots {
		batch.Queue(`insert into slots(id,room_id,start_at,end_at) values($1,$2,$3,$4) on conflict do nothing`, s.ID, s.RoomID, s.Start, s.End)
	}
	br := p.db.Pool().SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(slots); i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (p *Repository) ListAvailableSlotsByRoomAndDate(ctx context.Context, roomID uuid.UUID, date time.Time) ([]model.Slot, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	const q = `
	select s.id,s.room_id,s.start_at,s.end_at
	from slots s
	left join bookings b on b.slot_id=s.id and b.status='active'
	where s.room_id=$1 and s.start_at >= $2 and s.start_at < $3 and b.id is null
	order by s.start_at asc`
	rows, err := p.db.Pool().Query(ctx, q, roomID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var slots []model.Slot
	for rows.Next() {
		var s model.Slot
		if err = rows.Scan(&s.ID, &s.RoomID, &s.Start, &s.End); err != nil {
			return nil, err
		}
		slots = append(slots, s)
	}
	return slots, rows.Err()
}

func (p *Repository) GetSlotByID(ctx context.Context, slotID uuid.UUID) (model.Slot, error) {
	var s model.Slot
	err := p.db.Pool().QueryRow(ctx, `select id,room_id,start_at,end_at from slots where id=$1`, slotID).Scan(&s.ID, &s.RoomID, &s.Start, &s.End)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Slot{}, apperrors.NotFound
	}
	return s, err
}

func (p *Repository) CreateBooking(ctx context.Context, b model.Booking) (model.Booking, error) {
	const q = `insert into bookings(id,slot_id,user_id,status,conference_link,created_at) values($1,$2,$3,$4,$5,$6)`
	_, err := p.db.Pool().Exec(ctx, q, b.ID, b.SlotID, b.UserID, b.Status, b.ConferenceLink, b.CreatedAt)
	if err != nil {
		if uniqueError(err) {
			return model.Booking{}, apperrors.Conflict
		}
		return model.Booking{}, err
	}
	return b, nil
}

func (p *Repository) ListBookings(ctx context.Context, page, pageSize int) ([]model.Booking, int, error) {
	var total int
	if err := p.db.Pool().QueryRow(ctx, `select count(1) from bookings`).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := p.db.Pool().Query(ctx, `select id,slot_id,user_id,status,conference_link,created_at from bookings order by created_at desc limit $1 offset $2`, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]model.Booking, 0, pageSize)
	for rows.Next() {
		var b model.Booking
		if err = rows.Scan(&b.ID, &b.SlotID, &b.UserID, &b.Status, &b.ConferenceLink, &b.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
}

func (p *Repository) ListMyFutureBookings(ctx context.Context, userID uuid.UUID, now time.Time) ([]model.Booking, error) {
	const q = `
	select b.id,b.slot_id,b.user_id,b.status,b.conference_link,b.created_at
	from bookings b
	join slots s on s.id=b.slot_id
	where b.user_id=$1 and s.start_at >= $2
	order by s.start_at asc`
	rows, err := p.db.Pool().Query(ctx, q, userID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Booking
	for rows.Next() {
		var b model.Booking
		if err = rows.Scan(&b.ID, &b.SlotID, &b.UserID, &b.Status, &b.ConferenceLink, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (p *Repository) GetBooking(ctx context.Context, bookingID uuid.UUID) (model.Booking, error) {
	var b model.Booking
	err := p.db.Pool().QueryRow(ctx, `select id,slot_id,user_id,status,conference_link,created_at from bookings where id=$1`, bookingID).
		Scan(&b.ID, &b.SlotID, &b.UserID, &b.Status, &b.ConferenceLink, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Booking{}, apperrors.NotFound
	}
	return b, err
}

func (p *Repository) CancelBooking(ctx context.Context, bookingID uuid.UUID) error {
	_, err := p.db.Pool().Exec(ctx, `update bookings set status='cancelled' where id=$1`, bookingID)
	return err
}

func uniqueError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
