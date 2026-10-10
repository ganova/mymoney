package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Category adalah kategori pemasukan/pengeluaran.
type Category struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // expense | income
	Group    string `json:"group"`
	Icon     string `json:"icon"`
	Color    string `json:"color"`
	Keywords string `json:"keywords"`
	Archived bool   `json:"archived"`
	Sort     int    `json:"sort"`
}

const categoryCols = `id, name, kind, grp, icon, color, keywords, archived, sort`

func scanCategory(sc interface{ Scan(...any) error }) (Category, error) {
	var c Category
	err := sc.Scan(&c.ID, &c.Name, &c.Kind, &c.Group, &c.Icon, &c.Color, &c.Keywords, &c.Archived, &c.Sort)
	return c, err
}

// ListCategories mengembalikan semua kategori.
func (s *Store) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+categoryCols+` FROM categories ORDER BY archived, kind DESC, sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCategory mengambil satu kategori.
func (s *Store) GetCategory(ctx context.Context, id int64) (Category, error) {
	c, err := scanCategory(s.DB.QueryRowContext(ctx, `SELECT `+categoryCols+` FROM categories WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

func (c *Category) normalize() error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return invalid("nama kategori wajib diisi")
	}
	if c.Kind == "" {
		c.Kind = "expense"
	}
	if c.Kind != "expense" && c.Kind != "income" {
		return invalid("jenis kategori harus expense atau income")
	}
	c.Group = strings.TrimSpace(c.Group)
	if c.Group == "" {
		if c.Kind == "income" {
			c.Group = "Pemasukan"
		} else {
			c.Group = "Lain-lain"
		}
	}
	c.Keywords = normalizeList(c.Keywords)
	return nil
}

// CreateCategory menambah kategori.
func (s *Store) CreateCategory(ctx context.Context, c Category) (Category, error) {
	if err := c.normalize(); err != nil {
		return c, err
	}
	// hindari duplikat nama
	var existing int64
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM categories WHERE lower(name) = lower(?) AND kind = ? AND archived = 0`, c.Name, c.Kind).Scan(&existing); err == nil {
		return s.GetCategory(ctx, existing)
	}
	if c.Sort == 0 {
		s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort),0)+1 FROM categories`).Scan(&c.Sort)
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO categories (name, kind, grp, icon, color, keywords, archived, sort) VALUES (?,?,?,?,?,?,?,?)`,
		c.Name, c.Kind, c.Group, c.Icon, c.Color, c.Keywords, c.Archived, c.Sort)
	if err != nil {
		return c, err
	}
	id, _ := res.LastInsertId()
	return s.GetCategory(ctx, id)
}

// UpdateCategory memperbarui kategori.
func (s *Store) UpdateCategory(ctx context.Context, c Category) (Category, error) {
	if err := c.normalize(); err != nil {
		return c, err
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE categories SET name=?, kind=?, grp=?, icon=?, color=?, keywords=?, archived=?, sort=? WHERE id=?`,
		c.Name, c.Kind, c.Group, c.Icon, c.Color, c.Keywords, c.Archived, c.Sort, c.ID)
	if err != nil {
		return c, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return c, ErrNotFound
	}
	return s.GetCategory(ctx, c.ID)
}

// DeleteCategory menghapus kategori; transaksi terkait menjadi tanpa kategori.
func (s *Store) DeleteCategory(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM categories WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
