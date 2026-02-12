package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (store *Store) CreateInboxMessage(ctx context.Context, args InboxMessageCreateArgs) (InboxMessage, error) {
	if args.SenderThreadID <= 0 {
		return InboxMessage{}, errors.New("sender_thread_id is required")
	}
	if args.ReceiverThreadID <= 0 {
		return InboxMessage{}, errors.New("receiver_thread_id is required")
	}
	if strings.TrimSpace(args.Message) == "" {
		return InboxMessage{}, errors.New("message is required")
	}

	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return InboxMessage{}, err
	}
	defer tx.Rollback()

	now := nowTimestamp()
	result, err := tx.ExecContext(ctx,
		`INSERT INTO inbox_messages(sender_thread_id, receiver_thread_id, message, status, created_at, delivered_at)
		 VALUES(?, ?, ?, 'pending', ?, NULL)`,
		args.SenderThreadID, args.ReceiverThreadID, strings.TrimSpace(args.Message), now,
	)
	if err != nil {
		return InboxMessage{}, err
	}
	msgID, err := result.LastInsertId()
	if err != nil {
		return InboxMessage{}, err
	}

	if err := store.bumpVersionTx(ctx, tx); err != nil {
		return InboxMessage{}, err
	}

	row := tx.QueryRowContext(ctx,
		`SELECT id, sender_thread_id, receiver_thread_id, message, status, created_at, delivered_at
		   FROM inbox_messages WHERE id = ?`, msgID)
	msg, err := scanInboxMessage(row)
	if err != nil {
		return InboxMessage{}, err
	}
	if err := tx.Commit(); err != nil {
		return InboxMessage{}, err
	}
	return msg, nil
}

func (store *Store) ListPendingInboxMessages(ctx context.Context, receiverThreadID int64) ([]InboxMessage, error) {
	rows, err := store.database.QueryContext(ctx,
		`SELECT id, sender_thread_id, receiver_thread_id, message, status, created_at, delivered_at
		   FROM inbox_messages
		  WHERE receiver_thread_id = ? AND status = 'pending'
		  ORDER BY id ASC`, receiverThreadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]InboxMessage, 0)
	for rows.Next() {
		msg, scanErr := scanInboxMessage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (store *Store) ListInboxMessages(ctx context.Context, threadID int64) ([]InboxMessage, error) {
	rows, err := store.database.QueryContext(ctx,
		`SELECT id, sender_thread_id, receiver_thread_id, message, status, created_at, delivered_at
		   FROM inbox_messages
		  WHERE sender_thread_id = ? OR receiver_thread_id = ?
		  ORDER BY id ASC`, threadID, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]InboxMessage, 0)
	for rows.Next() {
		msg, scanErr := scanInboxMessage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (store *Store) MarkInboxMessageDelivered(ctx context.Context, messageID int64) (InboxMessage, error) {
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return InboxMessage{}, err
	}
	defer tx.Rollback()

	now := nowTimestamp()
	result, err := tx.ExecContext(ctx,
		`UPDATE inbox_messages SET status = 'delivered', delivered_at = ? WHERE id = ? AND status = 'pending'`,
		now, messageID)
	if err != nil {
		return InboxMessage{}, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return InboxMessage{}, fmt.Errorf("inbox message not found or already delivered: %d", messageID)
	}

	if err := store.bumpVersionTx(ctx, tx); err != nil {
		return InboxMessage{}, err
	}

	row := tx.QueryRowContext(ctx,
		`SELECT id, sender_thread_id, receiver_thread_id, message, status, created_at, delivered_at
		   FROM inbox_messages WHERE id = ?`, messageID)
	msg, err := scanInboxMessage(row)
	if err != nil {
		return InboxMessage{}, err
	}
	if err := tx.Commit(); err != nil {
		return InboxMessage{}, err
	}
	return msg, nil
}

func scanInboxMessage(scanner rowScanner) (InboxMessage, error) {
	var msg InboxMessage
	var deliveredAt sql.NullString
	err := scanner.Scan(
		&msg.ID,
		&msg.SenderThreadID,
		&msg.ReceiverThreadID,
		&msg.Message,
		&msg.Status,
		&msg.CreatedAt,
		&deliveredAt,
	)
	if err != nil {
		return InboxMessage{}, err
	}
	if deliveredAt.Valid {
		msg.DeliveredAt = &deliveredAt.String
	}
	return msg, nil
}
