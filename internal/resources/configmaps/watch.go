package configmaps

// The configmap watch. Values are dropped as they arrive: the list only
// counts entries; describe and yaml fetch a configmap on request.

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"github.com/fu/k7s/internal/kube"
)

// Row is one configmap without its values.
type Row struct {
	Namespace, Name string
	Entries         int // data and binaryData
	Created         time.Time
	Version         string // resourceVersion: changes whenever the data changes
}

func (r Row) Key() string { return r.Namespace + "/" + r.Name }

// Handlers receive updates; they are called from background goroutines.
type Handlers struct {
	Row    func(r Row, deleted bool)
	Synced func()
	Error  func(error)
}

// Watch watches the configmaps of ns matching selector. It returns
// immediately and stops when ctx is done.
func Watch(ctx context.Context, c *kube.Client, ns, selector string, h Handlers) error {
	factory := informers.NewSharedInformerFactoryWithOptions(c.Kube, 0, informers.WithNamespace(ns),
		informers.WithTweakListOptions(func(o *metav1.ListOptions) { o.LabelSelector = selector }))
	inf := factory.Core().V1().ConfigMaps().Informer()
	// Keep the entry names, drop the values (configmaps can be large).
	if err := inf.SetTransform(func(obj any) (any, error) {
		if cm, ok := obj.(*corev1.ConfigMap); ok {
			for k := range cm.Data {
				cm.Data[k] = ""
			}
			for k := range cm.BinaryData {
				cm.BinaryData[k] = nil
			}
		}
		return obj, nil
	}); err != nil {
		return err
	}
	if err := inf.SetWatchErrorHandler(func(_ *cache.Reflector, err error) { h.Error(err) }); err != nil {
		return err
	}
	emit := func(obj any, deleted bool) {
		if t, ok := obj.(cache.DeletedFinalStateUnknown); ok {
			obj = t.Obj
		}
		if cm, ok := obj.(*corev1.ConfigMap); ok {
			h.Row(Row{Namespace: cm.Namespace, Name: cm.Name, Entries: len(cm.Data) + len(cm.BinaryData),
				Created: cm.CreationTimestamp.Time, Version: cm.ResourceVersion}, deleted)
		}
	}
	_, err := inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { emit(obj, false) },
		UpdateFunc: func(_, obj any) { emit(obj, false) },
		DeleteFunc: func(obj any) { emit(obj, true) },
	})
	if err != nil {
		return err
	}
	factory.Start(ctx.Done())
	go func() {
		if cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
			h.Synced()
		}
	}()
	return nil
}
