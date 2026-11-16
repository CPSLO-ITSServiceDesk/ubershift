-- Run this once in Supabase: SQL Editor -> New query -> Run.
-- These tables store Slack schedule messages and claims for Vercel Functions.

create extension if not exists btree_gist;

create table if not exists public.autoshift_schedule_messages (
  channel_id text not null,
  message_ts text not null,
  schedule_date date not null,
  source_text text not null,
  created_at timestamptz not null default now(),
  primary key (channel_id, message_ts)
);

create table if not exists public.autoshift_shift_claims (
  id uuid primary key default gen_random_uuid(),
  channel_id text not null,
  message_ts text not null,
  open_shift_id text not null,
  start_time timestamptz not null,
  end_time timestamptz not null,
  slack_user_id text not null,
  created_at timestamptz not null default now(),
  check (start_time < end_time),
  foreign key (channel_id, message_ts)
    references public.autoshift_schedule_messages (channel_id, message_ts)
    on delete cascade,
  exclude using gist (
    channel_id with =,
    message_ts with =,
    open_shift_id with =,
    tstzrange(start_time, end_time, '[)') with &&
  )
);

create index if not exists autoshift_shift_claims_message_index
  on public.autoshift_shift_claims (channel_id, message_ts, start_time);

alter table public.autoshift_schedule_messages enable row level security;
alter table public.autoshift_shift_claims enable row level security;

-- The Vercel server uses SUPABASE_SERVICE_ROLE_KEY, which bypasses RLS.
-- Do not expose that key in the browser.
