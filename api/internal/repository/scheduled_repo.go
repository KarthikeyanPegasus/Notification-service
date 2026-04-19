package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/spidey/notification-service/internal/domain"
)

// ScheduledRepository handles scheduled notification persistence.
type ScheduledRepository struct {
	db *DB
}

func NewScheduledRepository(db *DB) *ScheduledRepository {
	return &ScheduledRepository{db: db}
}

func (r *ScheduledRepository) Create(ctx context.Context, s *domain.ScheduledNotification) error {
	vars, err := json.Marshal(s.TemplateVars)
	if err != nil {
		return fmt.Errorf("marshalling template vars: %w", err)
	}

	const q = `
		INSERT INTO scheduled_notifications
			(id, notification_id, user_id, channel, template_id, template_vars,
			 scheduled_at, original_at, cadence_workflow_id, cadence_run_id,
			 status, reschedule_count, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`

	_, err = r.db.Pool.Exec(ctx, q,
		s.ID, s.NotificationID, s.UserID, s.Channel, s.TemplateID, vars,
		s.ScheduledAt, s.OriginalAt, s.WorkflowID, s.RunID,
		s.Status, s.RescheduleCount, s.CreatedAt, s.UpdatedAt,
	)
	return err
}

func (r *ScheduledRepository) GetByNotificationID(ctx context.Context, notifID uuid.UUID) (*domain.ScheduledNotification, error) {
	const q = `
		SELECT id, notification_id, user_id, channel, template_id, template_vars,
		       scheduled_at, original_at, cadence_workflow_id, cadence_run_id,
		       status, reschedule_count, created_at, updated_at
		FROM scheduled_notifications WHERE notification_id=$1`

	return r.scanOne(r.db.Pool.QueryRow(ctx, q, notifID))
}

func (r *ScheduledRepository) UpdateSchedule(ctx context.Context, notifID uuid.UUID, newTime time.Time, newRunID string) error {
	const q = `
		UPDATE scheduled_notifications
		SET scheduled_at=$1, cadence_run_id=$2, reschedule_count=reschedule_count+1, updated_at=$3
		WHERE notification_id=$4`
	_, err := r.db.Pool.Exec(ctx, q, newTime, newRunID, time.Now(), notifID)
	return err
}

func (r *ScheduledRepository) UpdateStatus(ctx context.Context, notifID uuid.UUID, status domain.NotificationStatus) error {
	const q = `UPDATE scheduled_notifications SET status=$1, updated_at=$2 WHERE notification_id=$3`
	_, err := r.db.Pool.Exec(ctx, q, status, time.Now(), notifID)
	return err
}

// List returns pending scheduled notifications with optional pagination.
func (r *ScheduledRepository) List(ctx context.Context, userID *uuid.UUID, statuses []domain.NotificationStatus, page, pageSize int) ([]*domain.ScheduledNotification, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	args := []any{}
	where := "WHERE 1=1"
	idx := 1

	if userID != nil {
		where += fmt.Sprintf(" AND user_id=$%d", idx)
		args = append(args, *userID)
		idx++
	}
	if len(statuses) > 0 {
		where += fmt.Sprintf(" AND status = ANY($%d)", idx)
		args = append(args, statuses)
		idx++
	}

	var total int64
	if err := r.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM scheduled_notifications "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	dataQ := fmt.Sprintf(`
		SELECT id, notification_id, user_id, channel, template_id, template_vars,
		       scheduled_at, original_at, cadence_workflow_id, cadence_run_id,
		       status, reschedule_count, created_at, updated_at
		FROM scheduled_notifications %s
		ORDER BY scheduled_at ASC
		LIMIT $%d OFFSET $%d`, where, idx, idx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.Pool.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []*domain.ScheduledNotification
	for rows.Next() {
		s, err := r.scanRow(rows)
		if err != nil {
			return nil, 0, err
		}
		results = append(results, s)
	}
	return results, total, rows.Err()
}

func (r *ScheduledRepository) scanOne(row pgx.Row) (*domain.ScheduledNotification, error) {
	s := &domain.ScheduledNotification{}
	var varBytes []byte
	err := row.Scan(
		&s.ID, &s.NotificationID, &s.UserID, &s.Channel, &s.TemplateID, &varBytes,
		&s.ScheduledAt, &s.OriginalAt, &s.WorkflowID, &s.RunID,
		&s.Status, &s.RescheduleCount, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if len(varBytes) > 0 {
		_ = json.Unmarshal(varBytes, &s.TemplateVars)
	}
	return s, nil
}

func (r *ScheduledRepository) scanRow(rows pgx.Rows) (*domain.ScheduledNotification, error) {
	s := &domain.ScheduledNotification{}
	var varBytes []byte
	err := rows.Scan(
		&s.ID, &s.NotificationID, &s.UserID, &s.Channel, &s.TemplateID, &varBytes,
		&s.ScheduledAt, &s.OriginalAt, &s.WorkflowID, &s.RunID,
		&s.Status, &s.RescheduleCount, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(varBytes) > 0 {
		_ = json.Unmarshal(varBytes, &s.TemplateVars)
	}
	return s, nil
}
