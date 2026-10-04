package store

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct{ Pool *pgxpool.Pool }

func taskRow(row pgx.Row) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.OwnerID, &t.Title, &t.Done)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return t, err
}
func (p *Postgres) Create(ctx context.Context, owner int64, title string) (Task, error) {
	return taskRow(p.Pool.QueryRow(ctx, `INSERT INTO tasks(owner_id,title) VALUES($1,$2) RETURNING id,owner_id,title,done`, owner, title))
}
func (p *Postgres) Get(ctx context.Context, owner, id int64) (Task, error) {
	return taskRow(p.Pool.QueryRow(ctx, `SELECT id,owner_id,title,done FROM tasks WHERE owner_id=$1 AND id=$2`, owner, id))
}
func (p *Postgres) Update(ctx context.Context, owner, id int64, title string, done bool) (Task, error) {
	return taskRow(p.Pool.QueryRow(ctx, `UPDATE tasks SET title=$3,done=$4 WHERE owner_id=$1 AND id=$2 RETURNING id,owner_id,title,done`, owner, id, title, done))
}
func (p *Postgres) Delete(ctx context.Context, owner, id int64) error {
	tag, err := p.Pool.Exec(ctx, `DELETE FROM tasks WHERE owner_id=$1 AND id=$2`, owner, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
func (p *Postgres) List(ctx context.Context, owner int64) ([]Task, error) {
	rows, err := p.Pool.Query(ctx, `SELECT id,owner_id,title,done FROM tasks WHERE owner_id=$1 ORDER BY id`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []Task{}
	for rows.Next() {
		t, err := taskRow(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
