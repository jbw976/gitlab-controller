/*
Copyright 2019 The GitLab-Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gitlab

import (
	"context"
	"testing"

	xpcorev1alpha1 "github.com/crossplaneio/crossplane/pkg/apis/core/v1alpha1"
	xpstoragev1alpha1 "github.com/crossplaneio/crossplane/pkg/apis/storage/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/helm/pkg/chartutil"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplaneio/gitlab-controller/pkg/apis/controller/v1alpha1"
	"github.com/crossplaneio/gitlab-controller/pkg/test"
)

const testBucket = "test-bucket"

type mockSecretTransformer struct {
	mockTransform func(context.Context) error
}

func (m *mockSecretTransformer) transform(ctx context.Context) error {
	return m.mockTransform(ctx)
}

type mockSecretUpdater struct {
	mockUpdate func(*corev1.Secret) error
}

func (m *mockSecretUpdater) update(s *corev1.Secret) error {
	return m.mockUpdate(s)
}

type mockSecretDataCreator struct {
	mockCreate func(*corev1.Secret) error
}

func (m *mockSecretDataCreator) create(s *corev1.Secret) error { return m.mockCreate(s) }

func getBucketClaimType(name string) string {
	return bucketClaimKind + "-" + name
}

func assertBucketObject(t *testing.T, testName string, obj runtime.Object) *xpstoragev1alpha1.Bucket {
	bucket, ok := obj.(*xpstoragev1alpha1.Bucket)
	if !ok {
		t.Errorf("%s unexpected type: %T", testName, obj)
		return nil
	}
	return bucket
}

var _ resourceReconciler = &bucketReconciler{}

func Test_bucketReconciler_reconcile(t *testing.T) {
	ctx := context.TODO()
	testCaseName := "bucketReconciler.reconcile()"
	testError := errors.New("test-error")

	assertBucketName := func(obj runtime.Object) *xpstoragev1alpha1.Bucket {
		b := assertBucketObject(t, testCaseName, obj)
		if diff := cmp.Diff(b.Spec.Name, testName+"-"+xpstoragev1alpha1.BucketKind+"-"+testBucket+bucketNameDelimiter+"%s"); diff != "" {
			t.Errorf("%s unexpected name: %s", testCaseName, diff)
		}
		return b
	}

	type fields struct {
		gitlab      *v1alpha1.GitLab
		client      client.Client
		bucketName  string
		finder      resourceClassFinder
		transformer secretTransformer
	}
	type want struct {
		err    error
		status *xpcorev1alpha1.ResourceClaimStatus
	}
	tests := map[string]struct {
		fields fields
		want   want
	}{
		"FailToFindResourceClass": {
			fields: fields{
				gitlab: newGitLabBuilder().build(),
				finder: &mockResourceClassFinder{
					mockFind: func(ctx context.Context, provider corev1.ObjectReference,
						resource string) (*corev1.ObjectReference, error) {
						return nil, testError
					},
				},
				bucketName: testBucket,
			},
			want: want{
				err: errors.Wrapf(testError, errorFmtFailedToFindResourceClass, getBucketClaimType(testBucket), newGitLabBuilder().build().GetProviderRef()),
			},
		},
		"FailToCreate": {
			fields: fields{
				gitlab: newGitLabBuilder().withMeta(testMeta).build(),
				client: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
						assertBucketName(obj)
						return kerrors.NewNotFound(schema.GroupResource{}, "")
					},
					MockCreate: func(ctx context.Context, obj runtime.Object) error {
						assertBucketName(obj)
						return testError
					},
				},
				finder: &mockResourceClassFinder{
					mockFind: func(ctx context.Context, provider corev1.ObjectReference,
						resource string) (*corev1.ObjectReference, error) {
						return nil, nil
					},
				},
				bucketName: testBucket,
			},
			want: want{
				err: errors.Wrapf(testError, errorFmtFailedToCreate, getBucketClaimType(testBucket), testKey.String()+"-"+xpstoragev1alpha1.BucketKind+"-"+testBucket),
			},
		},
		"FailToRetrieveObject-Other": {
			fields: fields{
				gitlab: newGitLabBuilder().withMeta(testMeta).build(),
				client: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
						assertBucketName(obj)
						return testError
					},
				},
				finder: &mockResourceClassFinder{
					mockFind: func(ctx context.Context, provider corev1.ObjectReference,
						resource string) (*corev1.ObjectReference, error) {
						return nil, nil
					},
				},
				bucketName: testBucket,
			},
			want: want{err: errors.Wrapf(testError, errorFmtFailedToRetrieveInstance, getBucketClaimType(testBucket), testKey.String()+"-"+xpstoragev1alpha1.BucketKind+"-"+testBucket)},
		},
		"CreateSuccessful": {
			fields: fields{
				gitlab: newGitLabBuilder().withMeta(testMeta).build(),
				client: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
						assertBucketName(obj)
						return kerrors.NewNotFound(schema.GroupResource{}, "")
					},
					MockCreate: func(ctx context.Context, obj runtime.Object) error {
						assertBucketName(obj)
						return nil
					},
				},
				finder: &mockResourceClassFinder{
					mockFind: func(ctx context.Context, provider corev1.ObjectReference,
						resource string) (*corev1.ObjectReference, error) {
						return nil, nil
					},
				},
				bucketName: testBucket,
			},
			want: want{},
		},
		"SuccessfulNotReady": {
			fields: fields{
				gitlab: newGitLabBuilder().withMeta(testMeta).build(),
				client: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
						b := assertBucketName(obj)
						b.Status = *newResourceClaimStatusBuilder().withCreatingStatus().build()
						return nil
					},
					MockCreate: func(ctx context.Context, obj runtime.Object) error {
						assertBucketName(obj)
						return nil
					},
				},
				finder: &mockResourceClassFinder{
					mockFind: func(ctx context.Context, provider corev1.ObjectReference,
						resource string) (*corev1.ObjectReference, error) {
						return nil, nil
					},
				},
				bucketName: testBucket,
			},
			want: want{
				status: newResourceClaimStatusBuilder().withCreatingStatus().build(),
			},
		},
		"SuccessfulReady": {
			fields: fields{
				gitlab: newGitLabBuilder().withMeta(testMeta).build(),
				client: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
						b := assertBucketName(obj)
						b.Status = *newResourceClaimStatusBuilder().withReadyStatus().build()
						return nil
					},
					MockCreate: func(ctx context.Context, obj runtime.Object) error {
						assertBucketName(obj)
						return nil
					},
				},
				finder: &mockResourceClassFinder{
					mockFind: func(ctx context.Context, provider corev1.ObjectReference,
						resource string) (*corev1.ObjectReference, error) {
						return nil, nil
					},
				},
				transformer: &mockSecretTransformer{
					mockTransform: func(ctx context.Context) error { return nil },
				},
				bucketName: testBucket,
			},
			want: want{
				status: newResourceClaimStatusBuilder().withReadyStatus().build(),
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := newBucketReconciler(tt.fields.gitlab, tt.fields.client, tt.fields.bucketName, newMockHelmValuesFn(nil))
			r.resourceClassFinder = tt.fields.finder
			r.secretTransformer = tt.fields.transformer

			if diff := cmp.Diff(r.reconcile(ctx), tt.want.err, cmpErrors); diff != "" {
				t.Errorf("%s -got error, +want error: %s", testCaseName, diff)
			}
			if diff := cmp.Diff(r.status, tt.want.status, cmp.Comparer(test.EqualConditionedStatus)); diff != "" {
				t.Errorf("%s -got status, +want status: %s", testCaseName, diff)
			}
		})
	}
}

func Test_bucketReconciler_getClaimKind(t *testing.T) {
	r := newBucketReconciler(newGitLabBuilder().build(), test.NewMockClient(), testBucket, nil)
	if diff := cmp.Diff(r.getClaimKind(), getBucketClaimType(testBucket)); diff != "" {
		t.Errorf("bucketReconciler.getClaimKind() %s", diff)
	}
}

func Test_bucketReconciler_getHelmValues(t *testing.T) {
	type fields struct {
		baseResourceReconciler *baseResourceReconciler
		resourceClassFinder    resourceClassFinder
	}
	type args struct {
		ctx          context.Context
		values       chartutil.Values
		secretPrefix string
	}
	tests := map[string]struct {
		fields fields
		args   args
		want   error
	}{
		"Failure": {
			fields: fields{
				baseResourceReconciler: newBaseResourceReconciler(newGitLabBuilder().build(), test.NewMockClient(), testBucket),
			},
			args: args{ctx: context.TODO()},
			want: errors.New(errorResourceStatusIsNotFound),
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := &bucketReconciler{
				baseResourceReconciler: tt.fields.baseResourceReconciler,
				resourceClassFinder:    tt.fields.resourceClassFinder,
			}
			if diff := cmp.Diff(r.getHelmValues(tt.args.ctx, tt.args.values, tt.args.secretPrefix), tt.want, cmpErrors); diff != "" {
				t.Errorf("bucketReconciler.getHelmValues() error %s", diff)
			}
		})
	}
}

func Test_gitlabSecretTransformer_transform(t *testing.T) {
	ctx := context.TODO()
	testError := errors.New("test-error")
	testSecret := "test-secret"
	testSecretKey := types.NamespacedName{Namespace: testNamespace, Name: testSecret}
	type fields struct {
		baseResourceReconciler *baseResourceReconciler
		secretUpdaters         map[string]secretUpdater
	}
	tests := map[string]struct {
		fields  fields
		wantErr error
	}{
		"NoStatus": {
			fields: fields{
				baseResourceReconciler: &baseResourceReconciler{
					GitLab: newGitLabBuilder().build(),
				},
			},
			wantErr: errors.New(errorResourceStatusIsNotFound),
		},
		"FailedToRetrieveSecret": {
			fields: fields{
				baseResourceReconciler: &baseResourceReconciler{
					GitLab: newGitLabBuilder().withMeta(testMeta).build(),
					client: &test.MockClient{
						MockGet: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
							return testError
						},
					},
					status: newResourceClaimStatusBuilder().withCredentialsSecretRef(testSecret).build(),
				},
			},
			wantErr: errors.Wrapf(testError, errorFmtFailedToRetrieveConnectionSecret, testSecretKey),
		},
		"NotSupportedProvider": {
			fields: fields{
				baseResourceReconciler: &baseResourceReconciler{
					GitLab: newGitLabBuilder().withMeta(testMeta).build(),
					client: &test.MockClient{
						MockGet: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error { return nil },
					},
					status: newResourceClaimStatusBuilder().withCredentialsSecretRef(testSecret).build(),
				},
			},
			wantErr: errors.Errorf(errorFmtNotSupportedProvider, ""),
		},
		"UpdaterFailed": {
			fields: fields{
				baseResourceReconciler: &baseResourceReconciler{
					GitLab: newGitLabBuilder().withMeta(testMeta).build(),
					client: &test.MockClient{
						MockGet: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error { return nil },
					},
					status: newResourceClaimStatusBuilder().withCredentialsSecretRef(testSecret).build(),
				},
				secretUpdaters: map[string]secretUpdater{
					"": &mockSecretUpdater{
						mockUpdate: func(secret *corev1.Secret) error { return testError },
					},
				},
			},
			wantErr: errors.Wrapf(testError, errorFmtFailedToUpdateConnectionSecretData, testSecretKey),
		},
		"FailedToUpdateSecretObject": {
			fields: fields{
				baseResourceReconciler: &baseResourceReconciler{
					GitLab: newGitLabBuilder().withMeta(testMeta).build(),
					client: &test.MockClient{
						MockGet:    func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error { return nil },
						MockUpdate: func(ctx context.Context, obj runtime.Object) error { return testError },
					},
					status: newResourceClaimStatusBuilder().withCredentialsSecretRef(testSecret).build(),
				},
				secretUpdaters: map[string]secretUpdater{
					"": &mockSecretUpdater{
						mockUpdate: func(secret *corev1.Secret) error { return nil },
					},
				},
			},
			wantErr: errors.Wrapf(testError, errorFmtFailedToUpdateConnectionSecret, testSecretKey),
		},
		"Successful": {
			fields: fields{
				baseResourceReconciler: &baseResourceReconciler{
					GitLab: newGitLabBuilder().withMeta(testMeta).build(),
					client: &test.MockClient{
						MockGet:    func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error { return nil },
						MockUpdate: func(ctx context.Context, obj runtime.Object) error { return nil },
					},
					status: newResourceClaimStatusBuilder().withCredentialsSecretRef(testSecret).build(),
				},
				secretUpdaters: map[string]secretUpdater{
					"": &mockSecretUpdater{
						mockUpdate: func(secret *corev1.Secret) error { return nil },
					},
				},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tr := &gitLabSecretTransformer{
				baseResourceReconciler: tt.fields.baseResourceReconciler,
				secretUpdaters:         tt.fields.secretUpdaters,
			}
			if diff := cmp.Diff(tr.transform(ctx), tt.wantErr, cmpErrors); diff != "" {
				t.Errorf("gitLabSecretTransformer.transform() error %s", diff)
			}
		})
	}
}

func Test_bucketConnectionHelmValues(t *testing.T) {
	endpoint := "gcs://coolBucket"
	bucketName := "coolBucket"
	secretName := "coolSecret"
	secretPrefix := "coolPrefix-"
	credentials := "coolBucketCredentials"

	type args struct {
		values       chartutil.Values
		secret       *corev1.Secret
		name         string
		secretPrefix string
	}
	type want struct {
		values chartutil.Values
	}

	tests := map[string]struct {
		args args
		want want
	}{
		"EmptyValues": {
			args: args{
				values: chartutil.Values{},
				secret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: secretName},
					Data: map[string][]byte{
						xpcorev1alpha1.ResourceCredentialsSecretEndpointKey: []byte(endpoint),
						connectionKey: []byte(credentials),
					},
				},
				name:         bucketName,
				secretPrefix: secretPrefix,
			},
			want: want{
				values: chartutil.Values{
					valuesKeyGlobal: chartutil.Values{
						valuesKeyAppConfig: chartutil.Values{
							bucketName: chartutil.Values{
								"bucket": endpoint,
								"connection": chartutil.Values{
									"key":    connectionKey,
									"secret": secretPrefix + secretName,
								},
							},
						},
					},
				},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_ = bucketConnectionHelmValues(tt.args.values, tt.args.secret, tt.args.name, tt.args.secretPrefix)
			if diff := cmp.Diff(tt.want.values, tt.args.values); diff != "" {
				t.Errorf("bucketConnectionHelmValues() -want values, +got values: %s", diff)
			}
		})
	}
}

func Test_bucketBackupsHelmValues(t *testing.T) {
	endpoint := "gcs://coolBucket"
	bucketName := "coolBucket"
	secretName := "coolSecret"
	secretPrefix := "coolPrefix-"
	credentials := "coolBucketCredentials"

	type args struct {
		values       chartutil.Values
		secret       *corev1.Secret
		name         string
		secretPrefix string
	}
	type want struct {
		values chartutil.Values
	}
	tests := map[string]struct {
		args args
		want want
	}{
		"EmptyValues": {
			args: args{
				values: chartutil.Values{},
				secret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: secretName},
					Data: map[string][]byte{
						xpcorev1alpha1.ResourceCredentialsSecretEndpointKey: []byte(endpoint),
						connectionKey: []byte(credentials),
					},
				},
				name:         bucketName,
				secretPrefix: secretPrefix,
			},
			want: want{
				values: chartutil.Values{
					valuesKeyGitlab: chartutil.Values{
						"task-runner": chartutil.Values{
							"backups": chartutil.Values{
								"objectStorage": chartutil.Values{
									"config": chartutil.Values{
										"key":    connectionKey,
										"secret": secretPrefix + secretName,
									},
								},
							},
						},
					},
					valuesKeyGlobal: chartutil.Values{
						valuesKeyAppConfig: chartutil.Values{"coolBucket": chartutil.Values{"bucket": string("gcs://coolBucket")}},
					},
				},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_ = bucketBackupsHelmValues(tt.args.values, tt.args.secret, tt.args.name, tt.args.secretPrefix)
			if diff := cmp.Diff(tt.want.values, tt.args.values); diff != "" {
				t.Errorf("bucketBackupsHelmValues() -want values, +got values: %s", diff)
			}
		})
	}
}

func Test_bucketBackupsTempHelmValues(t *testing.T) {
	bucketName := "coolBucket"

	type args struct {
		values       chartutil.Values
		secret       *corev1.Secret
		name         string
		secretPrefix string
	}
	type want struct {
		values chartutil.Values
	}
	tests := map[string]struct {
		args args
		want want
	}{
		"EmptyValues": {
			args: args{
				values: chartutil.Values{},
				secret: &corev1.Secret{},
				name:   bucketName,
			},
			want: want{
				values: chartutil.Values{
					valuesKeyGlobal: chartutil.Values{
						valuesKeyAppConfig: chartutil.Values{
							"backups": chartutil.Values{"tmpBucket": bucketName},
						},
					},
				},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_ = bucketBackupsTempHelmValues(tt.args.values, tt.args.secret, tt.args.name, tt.args.secretPrefix)
			if diff := cmp.Diff(tt.want.values, tt.args.values); diff != "" {
				t.Errorf("bucketBackupsTempHelmValues() -want values, +got values: %s", diff)
			}
		})
	}
}
