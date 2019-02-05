package nodecerts

import (
	"crypto/x509"
	"net"
	"testing"

	"github.com/elastic/k8s-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/common/nodecerts/certutil"
	"github.com/elastic/k8s-operators/stack-operator/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_createValidatedCertificateTemplate(t *testing.T) {
	es := v1alpha1.ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-es-name",
			Namespace: "test-namespace",
		},
	}
	testIp := "1.2.3.4"
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod-name",
		},
		Status: corev1.PodStatus{
			PodIP: testIp,
		},
	}
	csr := x509.CertificateRequest{
		PublicKeyAlgorithm: x509.RSA,
		PublicKey:          &testRSAPrivateKey.PublicKey,
	}

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "2.2.3.3",
		},
	}

	validatedCert, err := createValidatedCertificateTemplate(pod, es.Name, es.Namespace, []corev1.Service{svc}, &csr)
	require.NoError(t, err)

	// roundtrip the certificate
	certRT, err := roundTripSerialize(validatedCert)
	require.NoError(t, err)
	require.NotNil(t, certRT, "roundtripped certificate should not be nil")

	// regular dns names and ip addresses should be present in the cert
	assert.Contains(t, certRT.DNSNames, pod.Name)
	assert.Contains(t, certRT.IPAddresses, net.ParseIP(testIp).To4())
	assert.Contains(t, certRT.IPAddresses, net.ParseIP("127.0.0.1").To4())

	// service ip and hosts should be present in the cert
	assert.Contains(t, certRT.IPAddresses, net.ParseIP(svc.Spec.ClusterIP).To4())
	assert.Contains(t, certRT.DNSNames, svc.Name)
	assert.Contains(t, certRT.DNSNames, getServiceFullyQualifiedHostname(svc))

	// es specific othernames is a bit more difficult to get to, but should be present:
	otherNames, err := certutil.ParseSANGeneralNamesOtherNamesOnly(certRT)
	require.NoError(t, err)

	// we expect this name to be used for both the common name as well as the es othername
	cn := "test-pod-name.node.test-es-name.test-namespace.es.cluster.local"

	otherName, err := (&certutil.UTF8StringValuedOtherName{
		OID:   certutil.CommonNameObjectIdentifier,
		Value: cn,
	}).ToOtherName()
	require.NoError(t, err)

	assert.Equal(t, certRT.Subject.CommonName, cn)
	assert.Contains(t, otherNames, certutil.GeneralName{OtherName: *otherName})
}

// roundTripSerialize does a serialization round-trip of the certificate in order to make sure any extra extensions
// are parsed and considered part of the certificate
func roundTripSerialize(cert *ValidatedCertificateTemplate) (*x509.Certificate, error) {
	certData, err := testCa.CreateCertificate(*cert)
	if err != nil {
		return nil, err
	}
	certRT, err := x509.ParseCertificate(certData)
	if err != nil {
		return nil, err
	}

	return certRT, nil
}

func TestEnsureNodeCertificateSecretExists(t *testing.T) {
	stubOwner := &corev1.Pod{}
	preExistingSecret := &corev1.Secret{}

	type args struct {
		c                   k8s.Client
		scheme              *runtime.Scheme
		owner               metav1.Object
		pod                 corev1.Pod
		nodeCertificateType string
		labels              map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    func(*testing.T, *corev1.Secret)
		wantErr bool
	}{
		{
			name: "should create a secret if it does not already exist",
			args: args{
				c:                   k8s.WrapClient(fake.NewFakeClient()),
				nodeCertificateType: LabelNodeCertificateTypeElasticsearchAll,
			},
			want: func(t *testing.T, secret *corev1.Secret) {
				assert.Contains(t, secret.Labels, LabelNodeCertificateType)
				assert.Equal(t, secret.Labels[LabelNodeCertificateType], LabelNodeCertificateTypeElasticsearchAll)
			},
		},
		{
			name: "should not create a new secret if it already exists",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient(preExistingSecret)),
			},
			want: func(t *testing.T, secret *corev1.Secret) {
				assert.Equal(t, preExistingSecret, secret)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.scheme == nil {
				tt.args.scheme = scheme.Scheme
			}

			if tt.args.owner == nil {
				tt.args.owner = stubOwner
			}

			got, err := EnsureNodeCertificateSecretExists(tt.args.c, tt.args.scheme, tt.args.owner, tt.args.pod, tt.args.nodeCertificateType, tt.args.labels)
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureNodeCertificateSecretExists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tt.want(t, got)
		})
	}
}
