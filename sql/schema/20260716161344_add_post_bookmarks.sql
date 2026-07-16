-- +goose Up
create table post_bookmarks (
    id UUID primary key,
    created_at timestamp not null,
    updated_at timestamp not null,
    user_id UUID not null references users(id) on delete cascade,
    post_id UUID not null references posts(id) on delete cascade,
    unique(user_id, post_id)
);

-- +goose Down
drop table post_bookmarks;
