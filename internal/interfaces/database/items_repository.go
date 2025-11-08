package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"Aicon-assignment/internal/domain/entity"
	domainErrors "Aicon-assignment/internal/domain/errors"
)

type ItemRepository struct {
	SqlHandler
}

func (r *ItemRepository) FindAll(ctx context.Context) ([]*entity.Item, error) {
	query := `
        SELECT id, name, category, brand, purchase_price, purchase_date, created_at, updated_at
        FROM items
        ORDER BY created_at DESC
    `

	rows, err := r.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", domainErrors.ErrDatabaseError, err.Error())
	}
	defer rows.Close()

	var items []*entity.Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", domainErrors.ErrDatabaseError, err.Error())
		}
		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %s", domainErrors.ErrDatabaseError, err.Error())
	}

	return items, nil
}

func (r *ItemRepository) FindByID(ctx context.Context, id int64) (*entity.Item, error) {
	query := `
        SELECT id, name, category, brand, purchase_price, purchase_date, created_at, updated_at
        FROM items
        WHERE id = ?
    `

	row := r.QueryRow(ctx, query, id)

	item, err := scanItem(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domainErrors.ErrItemNotFound
		}
		return nil, fmt.Errorf("%w: %s", domainErrors.ErrDatabaseError, err.Error())
	}

	return item, nil
}

func (r *ItemRepository) Create(ctx context.Context, item *entity.Item) (*entity.Item, error) {
	query := `
        INSERT INTO items (name, category, brand, purchase_price, purchase_date)
        VALUES (?, ?, ?, ?, ?)
    `

	result, err := r.Execute(ctx, query,
		item.Name,
		item.Category,
		item.Brand,
		item.PurchasePrice,
		item.PurchaseDate,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", domainErrors.ErrDatabaseError, err.Error())
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get last insert id: %s", domainErrors.ErrDatabaseError, err.Error())
	}

	return r.FindByID(ctx, id)
}

func (r *ItemRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM items WHERE id = ?`

	result, err := r.Execute(ctx, query, id)
	if err != nil {
		return fmt.Errorf("%w: %s", domainErrors.ErrDatabaseError, err.Error())
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%w: failed to get rows affected: %s", domainErrors.ErrDatabaseError, err.Error())
	}

	if rowsAffected == 0 {
		return domainErrors.ErrItemNotFound
	}

	return nil
}

func (r *ItemRepository) GetSummaryByCategory(ctx context.Context) (map[string]int, error) {
	query := `
        SELECT category, COUNT(*) as count
        FROM items
        GROUP BY category
    `

	rows, err := r.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", domainErrors.ErrDatabaseError, err.Error())
	}
	defer rows.Close()

	summary := make(map[string]int)
	for rows.Next() {
		var category string
		var count int
		if err := rows.Scan(&category, &count); err != nil {
			return nil, fmt.Errorf("%w: %s", domainErrors.ErrDatabaseError, err.Error())
		}
		summary[category] = count
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %s", domainErrors.ErrDatabaseError, err.Error())
	}

	return summary, nil
}

func scanItem(scanner interface {
	Scan(dest ...interface{}) error
}) (*entity.Item, error) {
	var item entity.Item
	var purchaseDate string
	var createdAt, updatedAt time.Time

	err := scanner.Scan(
		&item.ID,
		&item.Name,
		&item.Category,
		&item.Brand,
		&item.PurchasePrice,
		&purchaseDate,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if purchaseDate != "" {
		if parsedDate, err := time.Parse("2006-01-02", purchaseDate); err == nil {
			item.PurchaseDate = parsedDate.Format("2006-01-02")
		} else {
			item.PurchaseDate = purchaseDate
		}
	}

	item.CreatedAt = createdAt
	item.UpdatedAt = updatedAt

	return &item, nil
}

func (r *ItemRepository) UpdatePartially(ctx context.Context, id int64, item *entity.Item) (*entity.Item, error) {
	// どのフィールドを更新するか動的に決定
	setClauses := []string{}
	args := []interface{}{}

	if item.Name != "" {
		setClauses = append(setClauses, "name = ?")
		args = append(args, item.Name)
	}
	if item.Brand != "" {
		setClauses = append(setClauses, "brand = ?")
		args = append(args, item.Brand)
	}
	if item.PurchasePrice != 0 {
		setClauses = append(setClauses, "purchase_price = ?")
		args = append(args, item.PurchasePrice)
	}

	// 更新対象がない場合
	if len(setClauses) == 0 {
		return nil, fmt.Errorf("%w: no updatable fields provided", domainErrors.ErrDatabaseError)
	}

	// 更新時刻も更新
	setClauses = append(setClauses, "updated_at = CURRENT_TIMESTAMP")

	// SQL構築
	query := fmt.Sprintf("UPDATE items SET %s WHERE id = ?",
		joinClauses(setClauses, ", "))
	args = append(args, id)

	// 実行
	result, err := r.Execute(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", domainErrors.ErrDatabaseError, err.Error())
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get rows affected: %s", domainErrors.ErrDatabaseError, err.Error())
	}
	if rowsAffected == 0 {
		return nil, domainErrors.ErrItemNotFound
	}

	// 更新後のデータを再取得
	return r.FindByID(ctx, id)
}

func joinClauses(clauses []string, sep string) string {
	if len(clauses) == 0 {
		return ""
	}
	result := clauses[0]
	for i := 1; i < len(clauses); i++ {
		result += sep + clauses[i]
	}
	return result
}
