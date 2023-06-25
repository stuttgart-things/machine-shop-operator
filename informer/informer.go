package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"k8s.io/client-go/dynamic"

	sthingsK8s "github.com/stuttgart-things/sthingsK8s"
	batchv1 "k8s.io/api/batch/v1"

	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"

	"k8s.io/client-go/tools/cache"
)

var (
	wg sync.WaitGroup
)

func main() {

	clusterConfig, _ := sthingsK8s.GetKubeConfig(os.Getenv("KUBECONFIG"))
	clusterClient, err := dynamic.NewForConfig(clusterConfig)
	if err != nil {
		log.Fatalln(err)
	}

	kinds := []string{
		"jobs",
	}

	for i := range kinds {

		wg.Add(1)

		kind := kinds[i]
		namespace := "machine-shop"

		go func() {
			defer wg.Done()

			resource := schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: kind}

			factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(clusterClient, time.Minute, namespace, nil)
			informer := factory.ForResource(resource).Informer()

			mux := &sync.RWMutex{}
			synced := false
			informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					mux.RLock()
					defer mux.RUnlock()
					if !synced {
						return
					}

					createdUnstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
					fmt.Println(err)

					job := new(batchv1.Job)
					err = runtime.DefaultUnstructuredConverter.FromUnstructured(createdUnstructuredObj, &job)
					if err != nil {
						log.Fatal(err)
					}

					fmt.Println("JOB", job.Name)
					fmt.Println(kind, "created!")

					ctx := context.TODO()

					rdb := redis.NewClient(&redis.Options{ // no password set
						DB: 0, // use default DB
					})

					rdb.Set(ctx, "language", "Go", 1)

					err = rdb.Set(ctx, job.Name, "created", 0).Err()
					if err != nil {
						panic(err)
					}

					rdb.Close()

				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					mux.RLock()
					defer mux.RUnlock()
					if !synced {
						return
					}

					createdUnstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(newObj)
					fmt.Println(err)

					job := new(batchv1.Job)

					err = runtime.DefaultUnstructuredConverter.FromUnstructured(createdUnstructuredObj, &job)
					if err != nil {
						log.Fatal(err)
					}

					fmt.Println("NAME", job.Name)

					status := jobComplete(fmt.Sprintln(job.Status))

					if status["status"] != "True" {
						status["status"] = "running"
					} else {
						status["status"] = "finished"
					}

					fmt.Println("JOB", job.Name)
					fmt.Println(kind, status["status"])

					rdb := redis.NewClient(&redis.Options{
						DB: 0, // use default DB
					})

					ctx := context.TODO()
					rdb.Set(ctx, "language", "Go", 1)

					err = rdb.Set(ctx, job.Name, status["status"], 0).Err()
					if err != nil {
						panic(err)
					}

					rdb.Close()

				},
				// DeleteFunc: func(obj interface{}) {
				// 	mux.RLock()
				// 	defer mux.RUnlock()
				// 	if !synced {
				// 		return
				// 	}
				// 	fmt.Println("DELETED!")

				// },
			})

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			go informer.Run(ctx.Done())

			isSynced := cache.WaitForCacheSync(ctx.Done(), informer.HasSynced)
			mux.Lock()
			synced = isSynced
			mux.Unlock()

			if !isSynced {
				log.Fatal("failed to sync")
			}

			<-ctx.Done()

		}()

	}

	wg.Wait()

}

func jobComplete(prStatus string) (jobStatusMessage map[string]string) {

	jobStatusMessage = make(map[string]string)

	jobStatusMessage["status"], _ = GetRegexSubMatch(prStatus, `Complete\s(\w+)`)

	return

}

func GetRegexSubMatch(scanText, regexPattern string) (string, bool) {

	rgx := regexp.MustCompile(regexPattern)
	regexSubMatch := rgx.FindStringSubmatch(scanText)

	if len(regexSubMatch) == 0 {
		return "", false
	}

	return strings.Trim(regexSubMatch[1], " "), true
}
