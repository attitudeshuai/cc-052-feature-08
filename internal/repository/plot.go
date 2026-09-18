package repository

import (
	"cc-052/internal/model"

	"github.com/jmoiron/sqlx"
)

type PlotRepo struct {
	db *sqlx.DB
}

func NewPlotRepo(db *sqlx.DB) *PlotRepo {
	return &PlotRepo{db: db}
}

// Create 建地并在同一事务里写入初始归属期：从建地时刻起挂在创建方名下。
func (r *PlotRepo) Create(p *model.Plot) error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `INSERT INTO plot (farm_id, name, area_mu, geojson, soil_type)
	          VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`
	if err := tx.QueryRowx(query, p.FarmID, p.Name, p.AreaMu, p.GeoJSON, p.SoilType).
		Scan(&p.ID, &p.CreatedAt); err != nil {
		return err
	}

	if _, err := tx.Exec(`INSERT INTO plot_ownership (plot_id, farm_id, valid_from, valid_to)
		VALUES ($1, $2, $3, NULL)`, p.ID, p.FarmID, p.CreatedAt); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *PlotRepo) GetByID(id int64) (*model.Plot, error) {
	var p model.Plot
	query := `SELECT id, farm_id, name, area_mu, geojson, soil_type, created_at FROM plot WHERE id = $1`
	if err := r.db.Get(&p, query, id); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PlotRepo) ListByFarm(farmID int64) ([]model.Plot, error) {
	var plots []model.Plot
	query := `SELECT id, farm_id, name, area_mu, geojson, soil_type, created_at FROM plot WHERE farm_id = $1 ORDER BY id`
	if err := r.db.Select(&plots, query, farmID); err != nil {
		return nil, err
	}
	return plots, nil
}