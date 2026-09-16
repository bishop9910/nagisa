package data

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"nagisa/internal/biz"
	"nagisa/internal/data/ent/setting"
)

// settingRepo persists the runtime tunables.
type settingRepo struct {
	data *Data
}

// NewSettingRepo returns a setting repository.
func NewSettingRepo(d *Data) biz.SettingRepo { return &settingRepo{data: d} }

// ListSettings returns every stored key with its current value.
func (r *settingRepo) ListSettings(ctx context.Context) (map[string]string, error) {
	rows, err := r.data.Exec(ctx).Setting().Query().
		Select(setting.FieldKey, setting.FieldValue).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list settings: %w", err)
	}
	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.Value
	}
	return values, nil
}

// PutSettings upserts each key. The schema was generated without the upsert
// feature, so a write is an update followed by an insert when no row matched;
// both run in one transaction so the whole map is applied atomically.
func (r *settingRepo) PutSettings(ctx context.Context, values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	keys := slices.Sorted(maps.Keys(values))
	return r.data.WithTx(ctx, func(ctx context.Context) error {
		client := r.data.Exec(ctx).Setting()
		for _, key := range keys {
			value := values[key]
			updated, err := client.Update().
				Where(setting.Key(key)).
				SetValue(value).
				Save(ctx)
			if err != nil {
				return fmt.Errorf("data: update setting %q: %w", key, err)
			}
			if updated > 0 {
				continue
			}
			if _, err := client.Create().
				SetKey(key).
				SetValue(value).
				Save(ctx); err != nil {
				return fmt.Errorf("data: create setting %q: %w", key, err)
			}
		}
		return nil
	})
}
