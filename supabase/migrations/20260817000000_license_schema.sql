create extension if not exists pgcrypto;

drop table if exists activations;
drop table if exists licenses cascade;

create table licenses (
    id uuid primary key default gen_random_uuid(),
    license_key text unique not null,
    status text not null default 'active',
    hwid text,
    activated_at timestamptz,
    last_seen_at timestamptz,
    expires_at timestamptz,
    created_at timestamptz not null default now()
);

-- Index for fast lookups
create index idx_licenses_key on licenses(license_key);
