package controller

// healthChecker is an interface for checking health status
type healthChecker interface {
	IsHealthy() bool
}

// healthCheckers manages a collection of health checkers
type healthCheckers struct {
	checkers []healthChecker
}

// newHealthCheckers creates a new healthCheckers instance
func newHealthCheckers() *healthCheckers {
	return &healthCheckers{
		checkers: make([]healthChecker, 0),
	}
}

// addCheck adds a new health checker to the collection
func (c *healthCheckers) addCheck(checker healthChecker) {
	if checker != nil {
		c.checkers = append(c.checkers, checker)
	}
}

// isHealthy returns true if all registered checkers are healthy
func (c *healthCheckers) isHealthy() bool {
	for _, checker := range c.checkers {
		if !checker.IsHealthy() {
			return false
		}
	}
	return true
}
