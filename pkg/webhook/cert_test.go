package webhook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureSecret(t *testing.T) {
	t.Run("new secret", func(t *testing.T) {
		cl := fake.NewClientBuilder().Build()

		secretName := "secret-name"
		namespace := "namespace"
		certManager := &certManager{
			SecretName: secretName,
			Namespace:  namespace,
		}

		// prove secret doesn't exist
		secret := &corev1.Secret{}
		require.True(t, errors.IsNotFound(cl.Get(context.Background(), client.ObjectKey{Name: secretName, Namespace: namespace}, secret)), "expected secret to not exist")

		// prove ensuring secret creates the secret
		require.NoError(t, certManager.ensureSecret(context.Background(), cl), "expected no error ensuring the secret")
		require.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: secretName, Namespace: namespace}, secret), "expected no error getting the secret")
	})

	t.Run("existing secret", func(t *testing.T) {
		cl := fake.NewClientBuilder().Build()

		secretName := "secret-name"
		namespace := "namespace"
		certManager := &certManager{
			SecretName: secretName,
			Namespace:  namespace,
		}

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
		}

		require.NoError(t, cl.Create(context.Background(), secret), "expected no error creating secret")
		require.NoError(t, certManager.ensureSecret(context.Background(), cl), "expected no error ensuring the secret")
		require.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: secretName, Namespace: namespace}, secret), "expected no error getting the secret")
	})
}
