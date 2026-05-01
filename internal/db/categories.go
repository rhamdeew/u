package db

type Category struct {
	ID   int
	Name string
}

func (d *DB) ListCategories() ([]Category, error) {
	rows, err := d.sql.Query(`SELECT id, name FROM categories ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

func (d *DB) CreateCategory(name string) (*Category, error) {
	res, err := d.sql.Exec(`INSERT INTO categories (name) VALUES (?)`, name)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Category{ID: int(id), Name: name}, nil
}

func (d *DB) DeleteCategory(id int) error {
	_, err := d.sql.Exec(`DELETE FROM categories WHERE id = ?`, id)
	return err
}
