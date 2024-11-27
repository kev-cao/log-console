package mathutils

// FloorMod returns the floor modulus of x and y.
func FloorMod(x, y int) int {
	return (x%y + y) % y
}
