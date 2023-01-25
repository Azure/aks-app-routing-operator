package informer

import (
	"strconv"
	"testing"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	fakecgo "k8s.io/client-go/kubernetes/fake"
)

func TestIngressInformer(t *testing.T) {
	cs := fakecgo.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(cs, time.Duration(0))
	informer, err := NewIngress(f)
	require.NoError(t, err)

	// add ingresses
	cn := "class.name.com"
	ingsWithClassN := 4
	ingsWithClass := make(map[string]*netv1.Ingress)
	keyFn := func(i *netv1.Ingress) string {
		return i.Name
	}
	for i := 0; i < ingsWithClassN; i++ {
		ing := &netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: strconv.Itoa(i)},
			Spec:       netv1.IngressSpec{IngressClassName: util.StringPtr(cn)},
		}
		informer.Informer().GetIndexer().Add(ing)
		ingsWithClass[keyFn(ing)] = ing
	}

	// add other ingress
	otherCn := "other.class.com"
	otherIng := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "other"},
		Spec:       netv1.IngressSpec{IngressClassName: util.StringPtr(otherCn)},
	}
	informer.Informer().GetIndexer().Add(otherIng.DeepCopyObject())

	// prove that informer by classname returns all ingresses with a class
	ings, err := informer.ByIngressClassName(cn)
	require.NoError(t, err)
	require.True(t, len(ings) == ingsWithClassN)
	for _, ing := range ings {
		key := keyFn(ing)
		require.True(t, equality.Semantic.DeepEqual(ing, ingsWithClass[key]))
	}

	// update the other ingress to the same classname
	otherIng.Spec.IngressClassName = util.StringPtr(cn)
	informer.Informer().GetIndexer().Update(otherIng.DeepCopyObject())

	// prove that the informer returns the updated ingress
	ings, err = informer.ByIngressClassName(cn)
	require.NoError(t, err)
	require.True(t, len(ings) == ingsWithClassN+1)
	seen := false
	for _, ing := range ings {
		if equality.Semantic.DeepEqual(otherIng, ing) {
			seen = true
			break
		}
	}
	require.True(t, seen)
}
