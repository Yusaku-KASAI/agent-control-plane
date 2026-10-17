package filesession

// NewStoreWithoutFlock is a Store on a filesystem where flock does not exclude, like Docker
// Desktop's fakeowner bind mount, where a second flock on the same file is granted.
func NewStoreWithoutFlock(root string) *Store {
	s := NewStore(root)
	s.flock = func(int, int) error { return nil }
	return s
}
