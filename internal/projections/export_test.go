package projections

// EnsureColumnForTest exposes the migration helper only to tests.
func (rm *ReadModelsDB) EnsureColumnForTest(table, column, definition string) error {
	return rm.ensureColumn(table, column, definition)
}
