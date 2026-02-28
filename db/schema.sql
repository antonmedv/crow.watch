CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    username TEXT NOT NULL,
    email TEXT NOT NULL,
    password_digest TEXT NOT NULL,
    is_moderator BOOLEAN NOT NULL DEFAULT false,
    banned_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    inviter_id BIGINT REFERENCES users(id),
    campaign TEXT NOT NULL DEFAULT '',
    password_reset_token_hash TEXT,
    password_reset_token_created_at TIMESTAMPTZ,
    email_confirmed_at TIMESTAMPTZ,
    email_confirmation_token_hash TEXT,
    email_confirmation_token_created_at TIMESTAMPTZ,
    unconfirmed_email TEXT,
    website TEXT NOT NULL DEFAULT '',
    about TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_username_unique ON users (lower(username));
CREATE UNIQUE INDEX users_email_unique ON users (lower(email));
CREATE UNIQUE INDEX users_password_reset_token_hash_unique
  ON users (password_reset_token_hash) WHERE password_reset_token_hash IS NOT NULL;
CREATE UNIQUE INDEX users_email_confirmation_token_hash_unique
  ON users (email_confirmation_token_hash) WHERE email_confirmation_token_hash IS NOT NULL;

CREATE TABLE sessions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    ip_address TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX sessions_token_hash_unique ON sessions (token_hash);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE categories (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tags (
    id BIGSERIAL PRIMARY KEY,
    tag TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    category_id BIGINT REFERENCES categories(id),
    privileged BOOLEAN NOT NULL DEFAULT false,
    is_media BOOLEAN NOT NULL DEFAULT false,
    active BOOLEAN NOT NULL DEFAULT true,
    hotness_mod FLOAT NOT NULL DEFAULT 0.0 CHECK (hotness_mod >= -10 AND hotness_mod <= 10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX tags_tag_unique ON tags (lower(tag));

CREATE TABLE domains (
    id BIGSERIAL PRIMARY KEY,
    domain TEXT NOT NULL,
    banned BOOLEAN NOT NULL DEFAULT false,
    ban_reason TEXT NOT NULL DEFAULT '',
    story_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX domains_domain_unique ON domains (lower(domain));

CREATE TABLE origins (
    id BIGSERIAL PRIMARY KEY,
    domain_id BIGINT NOT NULL REFERENCES domains(id),
    origin TEXT NOT NULL,
    banned BOOLEAN NOT NULL DEFAULT false,
    ban_reason TEXT NOT NULL DEFAULT '',
    story_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX origins_origin_unique ON origins (lower(origin));

CREATE TABLE stories (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    domain_id BIGINT REFERENCES domains(id),
    origin_id BIGINT REFERENCES origins(id),
    url TEXT,
    normalized_url TEXT,
    title TEXT NOT NULL,
    body TEXT,
    short_code CHAR(6) NOT NULL,
    upvotes INT NOT NULL DEFAULT 0,
    downvotes INT NOT NULL DEFAULT 0,
    comment_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT stories_short_code_unique UNIQUE (short_code),
    CONSTRAINT stories_link_xor_text CHECK (
        (url IS NOT NULL AND normalized_url IS NOT NULL AND domain_id IS NOT NULL AND body IS NULL)
        OR
        (url IS NULL AND normalized_url IS NULL AND domain_id IS NULL AND body IS NOT NULL)
    )
);

CREATE INDEX stories_normalized_url_idx ON stories (normalized_url);
CREATE INDEX stories_created_at_idx ON stories (created_at);
CREATE INDEX stories_user_id_idx ON stories (user_id);

CREATE TABLE taggings (
    story_id BIGINT NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    tag_id BIGINT NOT NULL REFERENCES tags(id),
    PRIMARY KEY (story_id, tag_id)
);

CREATE INDEX taggings_tag_id_idx ON taggings (tag_id);

CREATE TABLE votes (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    story_id BIGINT NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, story_id)
);

CREATE TABLE hidden_tags (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tag_id  BIGINT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, tag_id)
);

CREATE TABLE comments (
    id BIGSERIAL PRIMARY KEY,
    story_id BIGINT NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    parent_id BIGINT REFERENCES comments(id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    depth INT NOT NULL DEFAULT 0 CHECK (depth >= 0 AND depth <= 10),
    upvotes INT NOT NULL DEFAULT 0,
    downvotes INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_comments_story_id ON comments(story_id);
CREATE INDEX idx_comments_user_id ON comments(user_id);
CREATE INDEX idx_comments_parent_id ON comments(parent_id);

CREATE TABLE comment_votes (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    comment_id BIGINT NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, comment_id)
);

CREATE TABLE comment_flags (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    comment_id BIGINT NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, comment_id)
);

CREATE TABLE hidden_stories (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    story_id BIGINT NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, story_id)
);

CREATE TABLE story_flags (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    story_id BIGINT NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, story_id)
);

CREATE TABLE story_visits (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    story_id BIGINT NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, story_id)
);

CREATE TABLE invitations (
    id         BIGSERIAL PRIMARY KEY,
    inviter_id BIGINT NOT NULL REFERENCES users(id),
    email      TEXT,
    token_hash TEXT NOT NULL,
    used_by_id BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX invitations_token_hash_unique ON invitations (token_hash);
CREATE INDEX invitations_inviter_id_idx ON invitations (inviter_id);

CREATE TABLE campaigns (
    id              BIGSERIAL PRIMARY KEY,
    slug            TEXT NOT NULL,
    welcome_message TEXT NOT NULL DEFAULT '',
    sponsor_id      BIGINT NOT NULL REFERENCES users(id),
    created_by_id   BIGINT NOT NULL REFERENCES users(id),
    active          BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX campaigns_slug_unique ON campaigns (lower(slug));
