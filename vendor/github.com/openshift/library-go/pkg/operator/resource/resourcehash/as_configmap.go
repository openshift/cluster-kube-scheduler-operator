package resourcehash

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/fnv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

// GetConfigMapHash returns a hash of the configmap data
func GetConfigMapHash(obj *corev1.ConfigMap) (string, error) {
	hasher := fnv.New32()
	if err := json.NewEncoder(hasher).Encode(obj.Data); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(hasher.Sum(nil)), nil
}

// GetSecretHash returns a hash of the secret data
func GetSecretHash(obj *corev1.Secret) (string, error) {
	hasher := fnv.New32()
	if err := json.NewEncoder(hasher).Encode(obj.Data); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(hasher.Sum(nil)), nil
}

// MultipleObjectHashStringMap returns a map of key/hash pairs suitable for merging into a configmap
func MultipleObjectHashStringMap(objs ...runtime.Object) (map[string]string, error) {
	ret := map[string]string{}

	for _, obj := range objs {
		switch t := obj.(type) {
		case *corev1.ConfigMap:
			hash, err := GetConfigMapHash(t)
			if err != nil {
				return nil, err
			}
			// this string coercion is lossy, but it should be fairly controlled and must be an allowed name
			ret[mapKeyFor("configmap", t.Namespace, t.Name)] = hash

		case *corev1.Secret:
			hash, err := GetSecretHash(t)
			if err != nil {
				return nil, err
			}
			// this string coercion is lossy, but it should be fairly controlled and must be an allowed name
			ret[mapKeyFor("secret", t.Namespace, t.Name)] = hash

		default:
			return nil, fmt.Errorf("%T is not handled", t)
		}
	}

	return ret, nil
}

func mapKeyFor(resource, namespace, name string) string {
	return fmt.Sprintf("%s.%s.%s", namespace, name, resource)
}

// ObjectReference can be used to reference a particular resource.  Not all group resources are respected by all methods.
type ObjectReference struct {
	Resource  schema.GroupResource
	Namespace string
	Name      string
}

// MultipleObjectHashStringMapForObjectReferences returns a map of key/hash pairs suitable for merging into a configmap
func MultipleObjectHashStringMapForObjectReferences(client kubernetes.Interface, objRefs ...ObjectReference) (map[string]string, error) {
	objs := []runtime.Object{}

	for _, objRef := range objRefs {
		switch objRef.Resource {
		case schema.GroupResource{Resource: "configmap"}, schema.GroupResource{Resource: "configmaps"}:
			obj, err := client.CoreV1().ConfigMaps(objRef.Namespace).Get(objRef.Name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			objs = append(objs, obj)

		case schema.GroupResource{Resource: "secret"}, schema.GroupResource{Resource: "secrets"}:
			obj, err := client.CoreV1().Secrets(objRef.Namespace).Get(objRef.Name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			objs = append(objs, obj)

		default:
			return nil, fmt.Errorf("%v is not handled", objRef.Resource)
		}
	}

	return MultipleObjectHashStringMap(objs...)
}
