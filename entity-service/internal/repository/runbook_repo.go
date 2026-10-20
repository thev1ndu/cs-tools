// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// RunbookRepository reads and writes the runbook catalog (migrations 0241, 0242).
type RunbookRepository interface {
	// List is every runbook with its alerts, ordered by name.
	List(ctx context.Context) ([]domain.Runbook, error)
	// Create adds a runbook; a ConflictError when its name or one of its alerts is taken.
	Create(ctx context.Context, actor string, req domain.SaveRunbookRequest) (domain.Runbook, error)
	// Update replaces a runbook and its alerts; nil when there is none with that id, a ConflictError when the name or an alert is taken.
	Update(ctx context.Context, actor, id string, req domain.SaveRunbookRequest) (*domain.Runbook, error)
	// Delete removes a runbook and its alerts; false when there was none.
	Delete(ctx context.Context, id string) (bool, error)
	// Import creates each runbook, or replaces the one with its name, in one transaction; an imported alert leaves any other runbook.
	Import(ctx context.Context, actor string, reqs []domain.SaveRunbookRequest) (domain.ImportRunbooksResponse, error)
}

type runbookRepo struct {
	db *pgxpool.Pool
}

// NewRunbookRepository constructs a Postgres-backed RunbookRepository.
func NewRunbookRepository(db *pgxpool.Pool) RunbookRepository {
	return &runbookRepo{db: db}
}

const runbookSelect = `SELECT r.id::text, r.name, r.runbook_url,
	COALESCE((SELECT array_agg(a.alert_name ORDER BY lower(a.alert_name)) FROM runbook_alert a WHERE a.runbook_id = r.id), '{}'),
	r.created_on, r.created_by, r.updated_on, r.updated_by
	FROM runbook r`

func scanRunbook(row pgx.Row) (domain.Runbook, error) {
	var rb domain.Runbook
	err := row.Scan(&rb.ID, &rb.Name, &rb.RunbookURL, &rb.Alerts, &rb.CreatedOn, &rb.CreatedBy, &rb.UpdatedOn, &rb.UpdatedBy)
	return rb, err
}

func (r *runbookRepo) List(ctx context.Context) ([]domain.Runbook, error) {
	rows, err := r.db.Query(ctx, runbookSelect+` ORDER BY lower(r.name), r.id`)
	if err != nil {
		return nil, fmt.Errorf("list runbooks: %w", err)
	}
	defer rows.Close()
	out := []domain.Runbook{}
	for rows.Next() {
		rb, err := scanRunbook(rows)
		if err != nil {
			return nil, fmt.Errorf("list runbooks: scan: %w", err)
		}
		out = append(out, rb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list runbooks: %w", err)
	}
	return out, nil
}

func (r *runbookRepo) Create(ctx context.Context, actor string, req domain.SaveRunbookRequest) (domain.Runbook, error) {
	var out domain.Runbook
	err := r.inTx(ctx, "create runbook", func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx,
			`INSERT INTO runbook (name, runbook_url, created_by, updated_by) VALUES ($1, $2, $3, $3) RETURNING id::text`,
			req.Name, req.RunbookURL, actor).Scan(&id); err != nil {
			if isUniqueViolation(err) {
				return &apierror.ConflictError{Msg: "a runbook named " + req.Name + " already exists"}
			}
			return err
		}
		if err := setRunbookAlertsTx(ctx, tx, id, req.Alerts, false); err != nil {
			return err
		}
		rb, err := scanRunbook(tx.QueryRow(ctx, runbookSelect+` WHERE r.id = $1::uuid`, id))
		out = rb
		return err
	})
	return out, err
}

func (r *runbookRepo) Update(ctx context.Context, actor, id string, req domain.SaveRunbookRequest) (*domain.Runbook, error) {
	var out *domain.Runbook
	err := r.inTx(ctx, "update runbook", func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE runbook SET name = $2, runbook_url = $3, updated_by = $4, updated_on = NOW() WHERE id = $1::uuid`,
			id, req.Name, req.RunbookURL, actor)
		if err != nil {
			if isUniqueViolation(err) {
				return &apierror.ConflictError{Msg: "a runbook named " + req.Name + " already exists"}
			}
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		if err := setRunbookAlertsTx(ctx, tx, id, req.Alerts, false); err != nil {
			return err
		}
		rb, err := scanRunbook(tx.QueryRow(ctx, runbookSelect+` WHERE r.id = $1::uuid`, id))
		out = &rb
		return err
	})
	return out, err
}

func (r *runbookRepo) Delete(ctx context.Context, id string) (bool, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM runbook WHERE id = $1::uuid`, id)
	if err != nil {
		return false, fmt.Errorf("delete runbook: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *runbookRepo) Import(ctx context.Context, actor string, reqs []domain.SaveRunbookRequest) (domain.ImportRunbooksResponse, error) {
	var out domain.ImportRunbooksResponse
	err := r.inTx(ctx, "import runbooks", func(tx pgx.Tx) error {
		for _, req := range reqs {
			var id string
			var inserted bool
			if err := tx.QueryRow(ctx,
				`INSERT INTO runbook (name, runbook_url, created_by, updated_by)
				 VALUES ($1, $2, $3, $3)
				 ON CONFLICT (lower(btrim(name))) DO UPDATE
				 SET runbook_url = EXCLUDED.runbook_url, updated_by = EXCLUDED.updated_by, updated_on = NOW()
				 RETURNING id::text, (xmax = 0)`,
				req.Name, req.RunbookURL, actor).Scan(&id, &inserted); err != nil {
				return err
			}
			if err := setRunbookAlertsTx(ctx, tx, id, req.Alerts, true); err != nil {
				return err
			}
			if inserted {
				out.Created++
			} else {
				out.Updated++
			}
		}
		return nil
	})
	return out, err
}

// setRunbookAlertsTx replaces a runbook's alerts; with steal an alert listed under another runbook moves here, otherwise it is a ConflictError.
func setRunbookAlertsTx(ctx context.Context, tx pgx.Tx, id string, alerts []string, steal bool) error {
	if _, err := tx.Exec(ctx, `DELETE FROM runbook_alert WHERE runbook_id = $1::uuid`, id); err != nil {
		return err
	}
	if steal {
		if _, err := tx.Exec(ctx, `DELETE FROM runbook_alert WHERE lower(btrim(alert_name)) = ANY($1::text[])`, lowerAll(alerts)); err != nil {
			return err
		}
	} else {
		var alert, owner string
		err := tx.QueryRow(ctx,
			`SELECT a.alert_name, r.name FROM runbook_alert a JOIN runbook r ON r.id = a.runbook_id
			 WHERE lower(btrim(a.alert_name)) = ANY($1::text[]) ORDER BY lower(a.alert_name) LIMIT 1`,
			lowerAll(alerts)).Scan(&alert, &owner)
		if err == nil {
			return &apierror.ConflictError{Msg: "alert " + alert + " already belongs to runbook " + owner}
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO runbook_alert (runbook_id, alert_name) SELECT $1::uuid, unnest($2::text[])`, id, alerts); err != nil {
		if isUniqueViolation(err) {
			return &apierror.ConflictError{Msg: "one of the alerts already belongs to another runbook"}
		}
		return err
	}
	return nil
}

// inTx runs fn in a transaction; a ConflictError passes through unwrapped so the handler can map it.
func (r *runbookRepo) inTx(ctx context.Context, op string, fn func(tx pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%s: begin tx: %w", op, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		var conflict *apierror.ConflictError
		if errors.As(err, &conflict) {
			return err
		}
		return fmt.Errorf("%s: %w", op, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%s: commit: %w", op, err)
	}
	return nil
}
