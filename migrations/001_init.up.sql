create table if not exists users (
    id uuid primary key,
    email text not null unique,
    password_hash text not null,
    role text not null,
    created_at timestamptz not null
);

create table if not exists rooms (
    id uuid primary key,
    name text not null,
    description text null,
    capacity integer null,
    created_at timestamptz not null
);

create table if not exists schedules (
    id uuid primary key,
    room_id uuid not null unique references rooms(id) on delete cascade,
    days_of_week jsonb not null,
    start_time text not null,
    end_time text not null,
    created_at timestamptz not null
);

create table if not exists slots (
    id uuid primary key,
    room_id uuid not null references rooms(id) on delete cascade,
    start_at timestamptz not null,
    end_at timestamptz not null,
    unique (room_id, start_at, end_at)
);

create table if not exists bookings (
    id uuid primary key,
    slot_id uuid not null references slots(id) on delete cascade,
    user_id uuid not null references users(id) on delete cascade,
    status text not null,
    conference_link text null,
    created_at timestamptz not null
);
CREATE UNIQUE INDEX idx_unique_active_booking ON bookings (slot_id) WHERE status = 'active';
CREATE INDEX idx_slots_room_date ON slots(room_id, start_at);
CREATE INDEX idx_bookings_user_id ON bookings(user_id);