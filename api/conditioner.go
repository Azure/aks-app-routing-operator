package api

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Conditioner interface {
	GetCondition(t string) *metav1.Condition
	GetConditions() *[]metav1.Condition
	GetGeneration() int64
}

func VerifyAndSetCondition(c Conditioner, condition metav1.Condition) {
	current := c.GetCondition(condition.Type)

	if current != nil && current.Status == condition.Status && current.Message == condition.Message && current.Reason == condition.Reason {
		current.ObservedGeneration = c.GetGeneration()
		return
	}

	condition.ObservedGeneration = c.GetGeneration()
	condition.LastTransitionTime = metav1.Now()
	meta.SetStatusCondition(c.GetConditions(), condition)
}
