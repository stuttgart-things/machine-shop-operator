package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	sthingsBase "github.com/stuttgart-things/sthingsBase"

	"github.com/fatih/color"
	goVersion "go.hein.dev/go-version"

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

	redisUrl      = os.Getenv("REDIS_SERVER") + ":" + os.Getenv("REDIS_PORT")
	redisPassword = os.Getenv("REDIS_PASSWORD")

	shortened = false
	version   = "unset"
	date      = "unknown"
	commit    = "unknown"
	output    = "yaml"
)

const banner = `
__  __  _____  ____       _____ _   _ ______ ____  _____  __  __ ______ _____
|  \/  |/ ____|/ __ \     |_   _| \ | |  ____/ __ \|  __ \|  \/  |  ____|  __ \
| \  / | (___ | |  | |______| | |  \| | |__ | |  | | |__) | \  / | |__  | |__) |
| |\/| |\___ \| |  | |______| | | . | |  __|| |  | |  _  /| |\/| |  __| |  _  /
| |  | |____) | |__| |     _| |_| |\  | |   | |__| | | \ \| |  | | |____| | \ \
|_|  |_|_____/ \____/     |_____|_| \_|_|    \____/|_|  \_|_|  |_|______|_|  \_\

`

func main() {

	// Output banner + version output
	color.Cyan(banner)
	resp := goVersion.FuncWithOutput(shortened, version, commit, date, output)
	color.Magenta(resp + "\n" + "REDIS-URL: " + redisUrl + "\n")

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

					rdb := redis.NewClient(&redis.Options{
						Addr:     redisUrl,
						Password: redisPassword, // no password set
						DB:       0,             // use default DB
					})

					rdb.Set(ctx, "language", "Go", 1000000)

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
						Addr:     redisUrl,
						Password: redisPassword, // no password set
						DB:       0,             // use default DB
					})

					ctx := context.TODO()
					rdb.Set(ctx, "language", "Go", 1000000)

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

	jobStatusMessage["status"], _ = sthingsBase.GetRegexSubMatch(prStatus, `Complete\s(\w+)`)

	return

}
