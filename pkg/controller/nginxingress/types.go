package nginxingress

// collision represents the type of collision that occurred when reconciling an nginxIngressController resource.
// This is used to determine the way we should handle the collision.
type collision int

const (
	collisionNone collision = iota
	collisionIngressClass
	collisionOther
)
