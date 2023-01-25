package informer

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	fakecgo "k8s.io/client-go/kubernetes/fake"
)

func TestIngressClassInformer(t *testing.T) {
	t.Run("can index by controller", func(t *testing.T) {
		cs := fakecgo.NewSimpleClientset()
		f := informers.NewSharedInformerFactory(cs, time.Duration(0))
		informer, err := NewIngressClass(f)
		require.NoError(t, err)
		require.NotNil(t, informer)

		// add ingressclasses
		controller := "example.com/controller"
		ingCsWithControllerN := 4
		ingCsWithController := make(map[string]*netv1.IngressClass)
		keyFn := func(i *netv1.IngressClass) string {
			return i.Name
		}
		for i := 0; i < ingCsWithControllerN; i++ {
			ingC := &netv1.IngressClass{
				ObjectMeta: metav1.ObjectMeta{Name: strconv.Itoa(i)},
				Spec:       netv1.IngressClassSpec{Controller: controller},
			}
			informer.Informer().GetIndexer().Add(ingC)
			ingCsWithController[keyFn(ingC)] = ingC
		}

		// add other ingressclass
		otherController := "other.com/controller"
		otherIngC := &netv1.IngressClass{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
			Spec:       netv1.IngressClassSpec{Controller: otherController},
		}
		informer.Informer().GetIndexer().Add(otherIngC.DeepCopyObject()) // deep copy because we update later

		// prove that informer by controller returns all ingressclasses with a controller
		ingCs, err := informer.ByController(controller)
		require.NoError(t, err)
		require.True(t, len(ingCs) == ingCsWithControllerN, fmt.Sprintf("ingressClasses length %d when expected %d", len(ingCs), ingCsWithControllerN))
		for _, ingC := range ingCs {
			key := keyFn(ingC)
			require.True(t, equality.Semantic.DeepEqual(ingC, ingCsWithController[key]), "ingressClass returned does not equal expected")
		}

		// update other ingressclass to the same controller
		otherIngC.Spec.Controller = controller
		informer.Informer().GetIndexer().Update(otherIngC)

		// prove that the informer returns the updated ingressclass
		ingCs, err = informer.ByController(controller)
		require.NoError(t, err)
		expectedLen := ingCsWithControllerN + 1
		require.True(t, len(ingCs) == expectedLen, fmt.Sprintf("ingressClasses length %d when expected %d", len(ingCs), expectedLen))
		seen := false
		for _, ingC := range ingCs {
			if equality.Semantic.DeepEqual(otherIngC, ingC) {
				seen = true
				break
			}
		}
		require.True(t, seen, "updated ingressclass not found")

		// delete all ingressclasses
		for _, ingC := range ingCs {
			informer.Informer().GetIndexer().Delete(ingC)
		}

		// prove that the informer returns no ingressclasses
		ingCs, err = informer.ByController(controller)
		require.NoError(t, err)
		require.True(t, len(ingCs) == 0, fmt.Sprintf("ingressClasses returned length %d when expected %d", len(ingCs), 0))
	})
}
