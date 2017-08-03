package migrations

var migration_v0003 = map[string][]string{
	"mysql": {
		`CREATE TABLE IF NOT EXISTS tags (
		tag int NOT NULL AUTO_INCREMENT,
		group_id int REFERENCES groups(id),
		PRIMARY KEY (tag)
	);`,
		`INSERT INTO tags (tag)
		SELECT (groups.id)
		FROM groups;`,
		`UPDATE tags SET group_id = tag;`,
	},
	"postgres": {
		`CREATE TABLE IF NOT EXISTS tags (
		tag SERIAL PRIMARY KEY,
		group_id int REFERENCES groups(id)
	);`,
		`INSERT INTO tags (tag)
		SELECT (groups.id)
		FROM groups;`,
		`UPDATE tags SET group_id = tag;`,
	},
}
