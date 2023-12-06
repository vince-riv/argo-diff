package argocd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/argoproj/gitops-engine/pkg/sync/hook"
	"github.com/argoproj/gitops-engine/pkg/sync/ignore"
	"github.com/argoproj/gitops-engine/pkg/utils/kube"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	cmdutil "github.com/argoproj/argo-cd/v2/cmd/util"
	"github.com/argoproj/argo-cd/v2/controller"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/settings"
	argoappv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	repoapiclient "github.com/argoproj/argo-cd/v2/reposerver/apiclient"
	"github.com/argoproj/argo-cd/v2/util/argo"
	argodiff "github.com/argoproj/argo-cd/v2/util/argo/diff"
)

// DifferenceOption struct to store diff options
type DifferenceOption struct {
	local         string
	localRepoRoot string
	revision      string
	cluster       *argoappv1.Cluster
	res           *repoapiclient.ManifestResponse
	serversideRes *repoapiclient.ManifestResponse
}

func GetApplicationDiff(ctx context.Context, appName, appNs, revision string) (ApplicationResourcesWithChanges, error) {

	appChanges := ApplicationResourcesWithChanges{}

	app, err := getApplication(ctx, appName, appNs)
	if err != nil {
		appChanges.WarnStr = fmt.Sprintf("Failed to refresh application %s: %s", appName, err.Error())
		return appChanges, err
	}
	appChanges.ArgoApp = app

	resources, err := getManagedResources(ctx, app.ObjectMeta.Name, app.ObjectMeta.Namespace)
	if err != nil {
		appChanges.WarnStr = fmt.Sprintf("Failed to get managed resources for application %s: %s", appName, err.Error())
		return appChanges, err
	}

	argoSettings, err := getSettings(ctx)
	if err != nil {
		appChanges.WarnStr = fmt.Sprintf("Failed to fetch ArgoCD Settings: %s", err.Error())
		return appChanges, err
	}

	res, err := getAppManifests(ctx, appName, appNs, revision)
	if err != nil {
		appChanges.WarnStr = fmt.Sprintf("Failed to fetch application manifests %s at revision %s: %s", appName, revision, err.Error())
		return appChanges, err
	}
	diffOption := &DifferenceOption{}
	diffOption.res = res
	diffOption.revision = revision
	proj, err := getProject(ctx, app.Spec.Project)
	if err != nil {
		appChanges.WarnStr = fmt.Sprintf("Failed to fetch project details %s for app %s: %s", app.Spec.Project, appName, err.Error())
		return appChanges, err
	}
	appResources, total, err := findDifferingObjects(ctx, app, proj.Project, resources, argoSettings, diffOption)
	// foundDiffs := findandPrintDiff(ctx, app, proj.Project, resources, argoSettings, diffOption)
	if err != nil {
		appChanges.WarnStr = fmt.Sprintf("Failed to generate diff data for %s for app: %s", appName, err.Error())
		return appChanges, err
	}
	appChanges.TotalObjectCount = total
	appChanges.ChangedResources = appResources
	return appChanges, nil
}

type resourceInfoProvider struct {
	namespacedByGk map[schema.GroupKind]bool
}

// Infer if obj is namespaced or not from corresponding live objects list. If corresponding live object has namespace then target object is also namespaced.
// If live object is missing then it does not matter if target is namespaced or not.
func (p *resourceInfoProvider) IsNamespaced(gk schema.GroupKind) (bool, error) {
	return p.namespacedByGk[gk], nil
}

func groupObjsByKey(localObs []*unstructured.Unstructured, liveObjs []*unstructured.Unstructured, appNamespace string) map[kube.ResourceKey]*unstructured.Unstructured {
	namespacedByGk := make(map[schema.GroupKind]bool)
	for i := range liveObjs {
		if liveObjs[i] != nil {
			key := kube.GetResourceKey(liveObjs[i])
			namespacedByGk[schema.GroupKind{Group: key.Group, Kind: key.Kind}] = key.Namespace != ""
		}
	}
	localObs, _, err := controller.DeduplicateTargetObjects(appNamespace, localObs, &resourceInfoProvider{namespacedByGk: namespacedByGk})
	objByKey := make(map[kube.ResourceKey]*unstructured.Unstructured)
	if err != nil {
		log.Error().Err(err).Msg("controller.DeduplicateTargetObjects failed")
		return objByKey
	}
	for i := range localObs {
		obj := localObs[i]
		if !(hook.IsHook(obj) || ignore.Ignore(obj)) {
			objByKey[kube.GetResourceKey(obj)] = obj
		}
	}
	return objByKey
}

type objKeyLiveTarget struct {
	key    kube.ResourceKey
	live   *unstructured.Unstructured
	target *unstructured.Unstructured
}

func findDifferingObjects(ctx context.Context, app *argoappv1.Application, proj *argoappv1.AppProject, resources *application.ManagedResourcesResponse, argoSettings *settings.Settings, diffOptions *DifferenceOption) ([]AppResource, int, error) {
	var appResources []AppResource
	totalResources := 0
	liveObjs, err := cmdutil.LiveObjects(resources.Items)
	if err != nil {
		log.Error().Err(err).Msg("cmdutil.LiveObjects() failed")
		return appResources, totalResources, err
	}
	items := make([]objKeyLiveTarget, 0)
	if diffOptions.revision != "" {
		var unstructureds []*unstructured.Unstructured
		for _, mfst := range diffOptions.res.Manifests {
			obj, err := argoappv1.UnmarshalToUnstructured(mfst)
			if err != nil {
				log.Error().Err(err).Msg("argoappv1.UnmarshalToUnstructured() failed")
				return appResources, totalResources, err
			}
			unstructureds = append(unstructureds, obj)
		}
		groupedObjs := groupObjsByKey(unstructureds, liveObjs, app.Spec.Destination.Namespace)
		items = groupObjsForDiff(resources, groupedObjs, items, argoSettings, app.InstanceName(argoSettings.ControllerNamespace), app.Spec.Destination.Namespace)
	} else {
		log.Fatal().Msg("Only diffOptions with revision set is supported")
	}

	for _, item := range items {
		if item.target != nil && hook.IsHook(item.target) || item.live != nil && hook.IsHook(item.live) {
			continue
		}
		totalResources++
		overrides := make(map[string]argoappv1.ResourceOverride)
		for k := range argoSettings.ResourceOverrides {
			val := argoSettings.ResourceOverrides[k]
			overrides[k] = *val
		}

		// TODO remove hardcoded IgnoreAggregatedRoles and retrieve the
		// compareOptions in the protobuf
		ignoreAggregatedRoles := false
		diffConfig, err := argodiff.NewDiffConfigBuilder().
			WithDiffSettings(app.Spec.IgnoreDifferences, overrides, ignoreAggregatedRoles).
			WithTracking(argoSettings.AppLabelKey, argoSettings.TrackingMethod).
			WithNoCache().
			Build()
		if err != nil {
			log.Error().Err(err).Msg("Failed to call argodiff.NewDiffConfigBuilder()")
			return appResources, totalResources, err
		}
		diffRes, err := argodiff.StateDiff(item.live, item.target, diffConfig)
		if err != nil {
			log.Error().Err(err).Msg("Failed to call argodiff.StateDiff()")
			return appResources, totalResources, err
		}

		if diffRes.Modified || item.target == nil || item.live == nil {
			r := AppResource{
				Group:     item.key.Group,
				Kind:      item.key.Kind,
				Namespace: item.key.Namespace,
				Name:      item.key.Name,
			}
			var live *unstructured.Unstructured
			var target *unstructured.Unstructured
			if item.target != nil && item.live != nil {
				target = &unstructured.Unstructured{}
				live = item.live
				err = json.Unmarshal(diffRes.PredictedLive, target)
				if err != nil {
					log.Error().Err(err).Msgf("Failed to json.Unmarshal() for predictedLive %s/%s %s/%s", item.key.Group, item.key.Kind, item.key.Namespace, item.key.Name)
				}
			} else {
				live = item.live
				target = item.target
			}
			if live == nil {
				r.YamlCur = ""
			} else {
				curObj, err := yaml.Marshal(live)
				if err != nil {
					log.Error().Err(err).Msgf("Failed to yaml.Unmarshal() for live state of %s/%s %s/%s", item.key.Group, item.key.Kind, item.key.Namespace, item.key.Name)
					r.YamlCur = "*** UNKNOWN LIVE STATE ***"
				} else {
					r.YamlCur = string(curObj)
				}

				newObj, err := yaml.Marshal(target)
				if err != nil {
					log.Error().Err(err).Msgf("Failed to yaml.Unmarshal() for target state of %s/%s %s/%s", item.key.Group, item.key.Kind, item.key.Namespace, item.key.Name)
					r.YamlNew = "*** UNKNOWN TARGET STATE ***"
				} else {
					r.YamlNew = string(newObj)
				}
			}
			appResources = append(appResources, r)
		}
	}
	return appResources, totalResources, nil
}

func groupObjsForDiff(resources *application.ManagedResourcesResponse, objs map[kube.ResourceKey]*unstructured.Unstructured, items []objKeyLiveTarget, argoSettings *settings.Settings, appName, namespace string) []objKeyLiveTarget {
	resourceTracking := argo.NewResourceTracking()
	var emptyReturn []objKeyLiveTarget
	for _, res := range resources.Items {
		var live = &unstructured.Unstructured{}
		err := json.Unmarshal([]byte(res.NormalizedLiveState), &live)
		if err != nil {
			log.Error().Err(err).Msg("json.Unmarshal() failed for res.NormalizedLiveState")
			return emptyReturn
		}

		key := kube.ResourceKey{Name: res.Name, Namespace: res.Namespace, Group: res.Group, Kind: res.Kind}
		if key.Kind == kube.SecretKind && key.Group == "" {
			// Don't bother comparing secrets, argo-cd doesn't have access to k8s secret data
			delete(objs, key)
			continue
		}
		if local, ok := objs[key]; ok || live != nil {
			if local != nil && !kube.IsCRD(local) {
				err = resourceTracking.SetAppInstance(local, argoSettings.AppLabelKey, appName, namespace, argoappv1.TrackingMethod(argoSettings.GetTrackingMethod()))
				if err != nil {
					log.Error().Err(err).Msg("resourceTracking.SetAppInstance() failed")
					return emptyReturn
				}
			}

			items = append(items, objKeyLiveTarget{key, live, local})
			delete(objs, key)
		}
	}
	for key, local := range objs {
		if key.Kind == kube.SecretKind && key.Group == "" {
			// Don't bother comparing secrets, argo-cd doesn't have access to k8s secret data
			delete(objs, key)
			continue
		}
		items = append(items, objKeyLiveTarget{key, nil, local})
	}
	return items
}
