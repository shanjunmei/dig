package gf_unused_provider

type Used struct{}
type Unused struct{}

func newUsed() *Used     { return &Used{} }
func newUnused() *Unused { return &Unused{} }
