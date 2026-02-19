package nono

type Destroyer struct {
	MetadataDir string
}

func (d *Destroyer) Destroy(name string) error {
	store := NewMetadataStore(d.MetadataDir, name)
	return store.Cleanup()
}
