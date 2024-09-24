package sliceutils

// Map applies a function to each element of a slice and returns a new slice with the results.
func Map[T, U any](f func(T, int) U, s []T) []U {
	result := make([]U, len(s))
	for i, v := range s {
		result[i] = f(v, i)
	}
	return result
}
