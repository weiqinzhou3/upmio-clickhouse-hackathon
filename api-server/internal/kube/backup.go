package kube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/model"
)

const (
	backupComponentLabel       = "app.kubernetes.io/component"
	backupComponentTask        = "clickhouse-backup-task"
	backupComponentSchedule    = "clickhouse-backup-schedule"
	backupTaskTypeLabel        = "upm.api/task.type"
	backupScheduleNameLabel    = "upm.api/backup.schedule-name"
	backupPathAnnotation       = "upm.api/backup.path"
	backupPathPrefixAnnotation = "upm.api/backup.path-prefix"
	backupSecretAnnotation     = "upm.api/backup.secret-ref"
	backupSourceAnnotation     = "upm.api/backup.source"
	backupTargetAnnotation     = "upm.api/backup.target"
	backupManagedByLabel       = "upm-api-server"

	backupStorageEndpointKey = "S3_ENDPOINT"
	backupStorageBucketKey   = "S3_BUCKET"
	backupStorageAccessKey   = "S3_ACCESS_KEY"
	backupStorageSecretKey   = "S3_SECRET_KEY"
	backupStorageUseSSLKey   = "S3_USE_SSL"
)

type backupRuntime struct {
	Namespace       string
	Name            string
	Image           string
	AdminSecretName string
	ClickHouseHost  string
}

func (s *Store) CreateBackup(ctx context.Context, namespace, name string, request model.BackupRequest) (model.TaskStatus, error) {
	runtime, err := s.backupRuntime(ctx, namespace, name)
	if err != nil {
		return model.TaskStatus{}, err
	}
	if err := s.validateBackupStorageSecret(ctx, namespace, request.Storage.SecretRef); err != nil {
		return model.TaskStatus{}, err
	}

	job := backupJob(runtime, model.TaskTypeBackup, request.Scope, model.RestoreTarget{}, request.Storage.SecretRef, request.Storage.Path, "")
	options := metav1.CreateOptions{}
	if request.DryRun {
		options.DryRun = []string{metav1.DryRunAll}
	}
	created, err := s.core.BatchV1().Jobs(namespace).Create(ctx, job, options)
	if err != nil {
		return model.TaskStatus{}, internalError("create backup Job", err)
	}
	return s.taskStatusFromJob(ctx, namespace, name, created, true)
}

func (s *Store) CreateRestore(ctx context.Context, namespace, name string, request model.RestoreRequest) (model.TaskStatus, error) {
	runtime, err := s.backupRuntime(ctx, namespace, name)
	if err != nil {
		return model.TaskStatus{}, err
	}
	if err := s.validateBackupStorageSecret(ctx, namespace, request.Storage.SecretRef); err != nil {
		return model.TaskStatus{}, err
	}

	job := backupJob(runtime, model.TaskTypeRestore, request.Source, request.Target, request.Storage.SecretRef, request.Storage.Path, "")
	options := metav1.CreateOptions{}
	if request.DryRun {
		options.DryRun = []string{metav1.DryRunAll}
	}
	created, err := s.core.BatchV1().Jobs(namespace).Create(ctx, job, options)
	if err != nil {
		return model.TaskStatus{}, internalError("create restore Job", err)
	}
	return s.taskStatusFromJob(ctx, namespace, name, created, true)
}

func (s *Store) GetTask(ctx context.Context, namespace, name, taskName string) (model.TaskStatus, error) {
	if _, err := s.GetCluster(ctx, namespace, name); err != nil {
		return model.TaskStatus{}, err
	}
	job, err := s.core.BatchV1().Jobs(namespace).Get(ctx, taskName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return model.TaskStatus{}, taskNotFoundError(namespace, taskName)
		}
		return model.TaskStatus{}, internalError("get backup task Job", err)
	}
	if job.Labels[serviceGroupNameLabel] != name || job.Labels[backupManagedByLabel] != "true" {
		return model.TaskStatus{}, taskNotFoundError(namespace, taskName)
	}
	return s.taskStatusFromJob(ctx, namespace, name, job, true)
}

func (s *Store) CreateBackupSchedule(ctx context.Context, namespace, name string, request model.BackupScheduleRequest) (model.BackupScheduleStatus, error) {
	runtime, err := s.backupRuntime(ctx, namespace, name)
	if err != nil {
		return model.BackupScheduleStatus{}, err
	}
	if err := s.validateBackupStorageSecret(ctx, namespace, request.Storage.SecretRef); err != nil {
		return model.BackupScheduleStatus{}, err
	}
	resourceName := backupScheduleResourceName(name, request.Name)
	if len(resourceName) > 52 {
		return model.BackupScheduleStatus{}, &model.APIError{
			Status:  http.StatusBadRequest,
			Code:    "VALIDATION_ERROR",
			Message: "backup schedule Kubernetes CronJob name must be 52 characters or fewer",
			Details: map[string]any{"resourceName": resourceName},
		}
	}

	cronJob := backupCronJob(runtime, request, resourceName)
	created, err := s.core.BatchV1().CronJobs(namespace).Create(ctx, cronJob, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return model.BackupScheduleStatus{}, &model.APIError{
				Status:  http.StatusConflict,
				Code:    "BACKUP_SCHEDULE_ALREADY_EXISTS",
				Message: "backup schedule already exists",
			}
		}
		return model.BackupScheduleStatus{}, internalError("create backup schedule CronJob", err)
	}
	return s.scheduleStatusFromCronJob(ctx, namespace, name, created)
}

func (s *Store) ListBackupSchedules(ctx context.Context, namespace, name string) ([]model.BackupScheduleStatus, error) {
	if _, err := s.GetCluster(ctx, namespace, name); err != nil {
		return nil, err
	}
	selector := labels.Set{
		serviceGroupNameLabel: name,
		backupComponentLabel:  backupComponentSchedule,
		backupManagedByLabel:  "true",
	}.AsSelector().String()
	cronJobs, err := s.core.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, internalError("list backup schedule CronJobs", err)
	}
	items := make([]model.BackupScheduleStatus, 0, len(cronJobs.Items))
	for i := range cronJobs.Items {
		status, err := s.scheduleStatusFromCronJob(ctx, namespace, name, &cronJobs.Items[i])
		if err != nil {
			return nil, err
		}
		items = append(items, status)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (s *Store) GetBackupSchedule(ctx context.Context, namespace, name, scheduleName string) (model.BackupScheduleStatus, error) {
	if _, err := s.GetCluster(ctx, namespace, name); err != nil {
		return model.BackupScheduleStatus{}, err
	}
	resourceName := backupScheduleResourceName(name, scheduleName)
	cronJob, err := s.core.BatchV1().CronJobs(namespace).Get(ctx, resourceName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return model.BackupScheduleStatus{}, backupScheduleNotFoundError(namespace, scheduleName)
		}
		return model.BackupScheduleStatus{}, internalError("get backup schedule CronJob", err)
	}
	if cronJob.Labels[serviceGroupNameLabel] != name || cronJob.Labels[backupScheduleNameLabel] != scheduleName {
		return model.BackupScheduleStatus{}, backupScheduleNotFoundError(namespace, scheduleName)
	}
	return s.scheduleStatusFromCronJob(ctx, namespace, name, cronJob)
}

func (s *Store) DeleteBackupSchedule(ctx context.Context, namespace, name, scheduleName string) error {
	if _, err := s.GetCluster(ctx, namespace, name); err != nil {
		return err
	}
	resourceName := backupScheduleResourceName(name, scheduleName)
	cronJob, err := s.core.BatchV1().CronJobs(namespace).Get(ctx, resourceName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return backupScheduleNotFoundError(namespace, scheduleName)
		}
		return internalError("get backup schedule CronJob before delete", err)
	}
	if cronJob.Labels[serviceGroupNameLabel] != name ||
		cronJob.Labels[backupScheduleNameLabel] != scheduleName ||
		cronJob.Labels[backupManagedByLabel] != "true" {
		return backupScheduleNotFoundError(namespace, scheduleName)
	}
	err = s.core.BatchV1().CronJobs(namespace).Delete(ctx, resourceName, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return backupScheduleNotFoundError(namespace, scheduleName)
		}
		return internalError("delete backup schedule CronJob", err)
	}
	return nil
}

func (s *Store) backupRuntime(ctx context.Context, namespace, name string) (backupRuntime, error) {
	cluster, err := s.GetCluster(ctx, namespace, name)
	if err != nil {
		return backupRuntime{}, err
	}
	server, err := s.dynamic.Resource(unitSetGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return backupRuntime{}, internalError("read ClickHouse UnitSet for backup", err)
	}
	adminSecret := server.GetAnnotations()[adminSecretAnnotation]
	if adminSecret == "" {
		return backupRuntime{}, &model.APIError{
			Status:  http.StatusUnprocessableEntity,
			Code:    "ADMIN_SECRET_NOT_FOUND",
			Message: "managed cluster does not record an admin Secret reference",
		}
	}
	if err := s.validateSecretKey(ctx, namespace, adminSecret, "CLICKHOUSE_ADMIN_PASSWORD", "ADMIN_SECRET"); err != nil {
		return backupRuntime{}, err
	}
	if err := s.validateSecretKey(ctx, namespace, "aes-secret-key", "AES_SECRET_KEY", "AES_SECRET"); err != nil {
		return backupRuntime{}, err
	}
	_, serverPods, err := s.listClusterPods(ctx, namespace, name)
	if err != nil {
		return backupRuntime{}, internalError("list ClickHouse Pods for backup", err)
	}
	pod, ok := firstPod(serverPods)
	if !ok {
		return backupRuntime{}, &model.APIError{
			Status:  http.StatusUnprocessableEntity,
			Code:    "CLICKHOUSE_POD_NOT_AVAILABLE",
			Message: "no ClickHouse Server Pod is available for backup or restore",
		}
	}
	image := ""
	for _, container := range pod.Spec.Containers {
		if container.Name == clickHouseContainerName {
			image = container.Image
			break
		}
	}
	if image == "" {
		return backupRuntime{}, &model.APIError{
			Status:  http.StatusUnprocessableEntity,
			Code:    "CLICKHOUSE_IMAGE_NOT_FOUND",
			Message: "ClickHouse container image could not be discovered from server Pod",
		}
	}
	return backupRuntime{
		Namespace:       namespace,
		Name:            cluster.Name,
		Image:           image,
		AdminSecretName: adminSecret,
		ClickHouseHost:  fmt.Sprintf("%s-0-svc.%s.svc", name, namespace),
	}, nil
}

func (s *Store) validateBackupStorageSecret(ctx context.Context, namespace, secretName string) error {
	for _, key := range []string{backupStorageEndpointKey, backupStorageBucketKey, backupStorageAccessKey, backupStorageSecretKey} {
		if err := s.validateSecretKey(ctx, namespace, secretName, key, "BACKUP_STORAGE_SECRET"); err != nil {
			return err
		}
	}
	return nil
}

func backupJob(runtime backupRuntime, taskType string, source model.BackupScope, target model.RestoreTarget, secretRef, backupPath, scheduleName string) *batchv1.Job {
	labels := backupTaskLabels(runtime.Name, taskType)
	if scheduleName != "" {
		labels[backupScheduleNameLabel] = scheduleName
	}
	annotations := backupTaskAnnotations(source, target, secretRef, backupPath, "")
	annotations[adminSecretAnnotation] = runtime.AdminSecretName
	backoffLimit := int32(0)
	ttl := int32(86400)
	activeDeadline := int64(900)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    runtime.Namespace,
			GenerateName: runtime.Name + "-" + taskType + "-",
			Labels:       labels,
			Annotations:  annotations,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttl,
			ActiveDeadlineSeconds:   &activeDeadline,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels, Annotations: annotations},
				Spec:       backupPodSpec(runtime, taskType, source, target, secretRef, backupPath, ""),
			},
		},
	}
}

func backupCronJob(runtime backupRuntime, request model.BackupScheduleRequest, resourceName string) *batchv1.CronJob {
	labels := backupTaskLabels(runtime.Name, model.TaskTypeBackup)
	labels[backupComponentLabel] = backupComponentSchedule
	labels[backupScheduleNameLabel] = request.Name
	annotations := backupTaskAnnotations(request.Scope, model.RestoreTarget{}, request.Storage.SecretRef, "", request.Storage.PathPrefix)
	annotations[adminSecretAnnotation] = runtime.AdminSecretName
	concurrency := batchv1.ConcurrencyPolicy(request.Execution.ConcurrencyPolicy)
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        resourceName,
			Namespace:   runtime.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   request.Schedule,
			TimeZone:                   emptyStringPointer(request.TimeZone),
			ConcurrencyPolicy:          concurrency,
			Suspend:                    &request.Suspend,
			SuccessfulJobsHistoryLimit: request.Execution.SuccessfulJobsHistoryLimit,
			FailedJobsHistoryLimit:     request.Execution.FailedJobsHistoryLimit,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      backupCronJobChildLabels(labels),
					Annotations: annotations,
				},
				Spec: batchv1.JobSpec{
					BackoffLimit:          int32Pointer(0),
					ActiveDeadlineSeconds: int64Pointer(900),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      backupCronJobChildLabels(labels),
							Annotations: annotations,
						},
						Spec: backupPodSpec(runtime, model.TaskTypeBackup, request.Scope, model.RestoreTarget{}, request.Storage.SecretRef, "", request.Storage.PathPrefix),
					},
				},
			},
		},
	}
}

func backupPodSpec(runtime backupRuntime, taskType string, source model.BackupScope, target model.RestoreTarget, secretRef, backupPath, backupPathPrefix string) corev1.PodSpec {
	return corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever,
		Containers: []corev1.Container{{
			Name:            "clickhouse-task",
			Image:           runtime.Image,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"bash", "-ec", backupJobScript()},
			Env: []corev1.EnvVar{
				{Name: "OPERATION", Value: taskType},
				{Name: "CLICKHOUSE_HOST", Value: runtime.ClickHouseHost},
				{Name: "CLICKHOUSE_TCP_PORT", Value: "9000"},
				{Name: "CLICKHOUSE_ADMIN_USER", Value: "admin"},
				{Name: "CLICKHOUSE_ADMIN_PASSWORD_KEY", Value: "CLICKHOUSE_ADMIN_PASSWORD"},
				{Name: "SECRET_MOUNT", Value: "/etc/upm/secret"},
				{Name: "SOURCE_DATABASE", Value: source.Database},
				{Name: "SOURCE_TABLE", Value: source.Table},
				{Name: "TARGET_DATABASE", Value: target.Database},
				{Name: "TARGET_TABLE", Value: target.Table},
				{Name: "BACKUP_PATH", Value: backupPath},
				{Name: "BACKUP_PATH_PREFIX", Value: backupPathPrefix},
				{Name: "AES_SECRET_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "aes-secret-key"}, Key: "AES_SECRET_KEY"}}},
				{Name: backupStorageEndpointKey, ValueFrom: backupSecretKeySelector(secretRef, backupStorageEndpointKey)},
				{Name: backupStorageBucketKey, ValueFrom: backupSecretKeySelector(secretRef, backupStorageBucketKey)},
				{Name: backupStorageAccessKey, ValueFrom: backupSecretKeySelector(secretRef, backupStorageAccessKey)},
				{Name: backupStorageSecretKey, ValueFrom: backupSecretKeySelector(secretRef, backupStorageSecretKey)},
				{Name: backupStorageUseSSLKey, ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secretRef}, Key: backupStorageUseSSLKey, Optional: boolPointer(true)}}},
			},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      "unit-secret",
				MountPath: "/etc/upm/secret",
				ReadOnly:  true,
			}},
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: boolPointer(false),
				Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			},
		}},
		Volumes: []corev1.Volume{{
			Name: "unit-secret",
			VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
				SecretName: runtime.AdminSecretName,
			}},
		}},
	}
}

func (s *Store) taskStatusFromJob(ctx context.Context, namespace, clusterName string, job *batchv1.Job, includeLogs bool) (model.TaskStatus, error) {
	taskType := job.Labels[backupTaskTypeLabel]
	status := model.TaskStatus{
		Name:      job.Name,
		Namespace: namespace,
		Cluster:   clusterName,
		Type:      taskType,
		Status:    taskStatusFromJobStatus(job),
		Message:   taskMessageFromJob(job),
		JobRef:    &model.TaskRef{Namespace: namespace, Name: job.Name},
		Evidence: map[string]any{
			"backupPath":       job.Annotations[backupPathAnnotation],
			"backupPathPrefix": job.Annotations[backupPathPrefixAnnotation],
			"source":           job.Annotations[backupSourceAnnotation],
			"target":           job.Annotations[backupTargetAnnotation],
			"storageSecretRef": job.Annotations[backupSecretAnnotation],
			"logsRedacted":     true,
		},
	}
	if job.Status.StartTime != nil {
		value := job.Status.StartTime.Time.UTC()
		status.StartTime = &value
	}
	if job.Status.CompletionTime != nil {
		value := job.Status.CompletionTime.Time.UTC()
		status.CompletionTime = &value
	}
	pod, logs, err := s.taskPodAndLogs(ctx, namespace, job.Name, includeLogs, job.Annotations[backupSecretAnnotation], job.Annotations[adminSecretAnnotation])
	if err != nil {
		status.Evidence["logError"] = errString(err)
		return status, nil
	}
	if pod != nil {
		status.PodRef = &model.TaskRef{Namespace: namespace, Name: pod.Name}
	}
	if logs != "" {
		status.Evidence["logTail"] = logs
	}
	return status, nil
}

func (s *Store) taskPodAndLogs(ctx context.Context, namespace, jobName string, includeLogs bool, storageSecret, adminSecret string) (*corev1.Pod, string, error) {
	pods, err := s.core.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set{"job-name": jobName}.AsSelector().String(),
	})
	if err != nil {
		return nil, "", internalError("list backup task Pods", err)
	}
	if len(pods.Items) == 0 {
		pods, err = s.core.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labels.Set{"batch.kubernetes.io/job-name": jobName}.AsSelector().String(),
		})
		if err != nil {
			return nil, "", internalError("list backup task Pods", err)
		}
	}
	if len(pods.Items) == 0 {
		return nil, "", nil
	}
	sort.Slice(pods.Items, func(i, j int) bool { return pods.Items[i].CreationTimestamp.Before(&pods.Items[j].CreationTimestamp) })
	pod := pods.Items[len(pods.Items)-1]
	if !includeLogs {
		return &pod, "", nil
	}
	request := s.core.CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{TailLines: int64Pointer(80)})
	reader, err := request.Stream(ctx)
	if err != nil {
		return &pod, "", internalError("read backup task logs", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return &pod, "", internalError("read backup task logs", err)
	}
	sensitive, sensitiveErr := s.collectTaskSensitiveValues(ctx, namespace, storageSecret, adminSecret)
	if sensitiveErr != nil {
		return &pod, trimEvidence(string(data)), sensitiveErr
	}
	return &pod, trimEvidence(redactString(string(data), sensitive)), nil
}

func (s *Store) collectTaskSensitiveValues(ctx context.Context, namespace, storageSecret, adminSecret string) ([]string, error) {
	secretNames := []string{"aes-secret-key"}
	if storageSecret != "" {
		secretNames = append(secretNames, storageSecret)
	}
	if adminSecret != "" {
		secretNames = append(secretNames, adminSecret)
	}
	values := []string{}
	for _, secretName := range secretNames {
		secret, err := s.core.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			return values, fmt.Errorf("read task secret for redaction: %w", err)
		}
		for _, value := range secret.Data {
			if len(value) >= 4 {
				values = append(values, string(value))
			}
		}
	}
	return values, nil
}

func (s *Store) scheduleStatusFromCronJob(ctx context.Context, namespace, clusterName string, cronJob *batchv1.CronJob) (model.BackupScheduleStatus, error) {
	active := make([]model.TaskRef, 0, len(cronJob.Status.Active))
	for _, ref := range cronJob.Status.Active {
		active = append(active, model.TaskRef{Namespace: namespace, Name: ref.Name})
	}
	status := model.BackupScheduleStatus{
		Name:                 cronJob.Labels[backupScheduleNameLabel],
		Namespace:            namespace,
		Cluster:              clusterName,
		Schedule:             cronJob.Spec.Schedule,
		Suspend:              cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend,
		ActiveJobs:           active,
		StorageSecretRef:     cronJob.Annotations[backupSecretAnnotation],
		BackupPathPrefix:     cronJob.Annotations[backupPathPrefixAnnotation],
		SuccessfulJobsRetain: pointerInt32Value(cronJob.Spec.SuccessfulJobsHistoryLimit),
		FailedJobsRetain:     pointerInt32Value(cronJob.Spec.FailedJobsHistoryLimit),
	}
	if cronJob.Spec.TimeZone != nil {
		status.TimeZone = *cronJob.Spec.TimeZone
	}
	if cronJob.Status.LastScheduleTime != nil {
		value := cronJob.Status.LastScheduleTime.Time.UTC()
		status.LastScheduleTime = &value
	}
	if cronJob.Status.LastSuccessfulTime != nil {
		value := cronJob.Status.LastSuccessfulTime.Time.UTC()
		status.LastSuccessfulTime = &value
	}
	jobs, err := s.scheduledBackupJobs(ctx, namespace, clusterName, status.Name)
	if err != nil {
		return model.BackupScheduleStatus{}, err
	}
	for i := range jobs {
		task, err := s.taskStatusFromJob(ctx, namespace, clusterName, &jobs[i], true)
		if err != nil {
			return model.BackupScheduleStatus{}, err
		}
		status.RecentJobs = append(status.RecentJobs, task)
	}
	return status, nil
}

func (s *Store) scheduledBackupJobs(ctx context.Context, namespace, clusterName, scheduleName string) ([]batchv1.Job, error) {
	selector := labels.Set{
		serviceGroupNameLabel:   clusterName,
		backupScheduleNameLabel: scheduleName,
		backupManagedByLabel:    "true",
	}.AsSelector().String()
	list, err := s.core.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, internalError("list scheduled backup Jobs", err)
	}
	jobs := list.Items
	sort.Slice(jobs, func(i, j int) bool { return jobs[j].CreationTimestamp.Before(&jobs[i].CreationTimestamp) })
	if len(jobs) > 5 {
		jobs = jobs[:5]
	}
	return jobs, nil
}

func backupTaskLabels(clusterName, taskType string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":    "upm-clickhouse-task",
		"app.kubernetes.io/part-of": "upmio",
		backupComponentLabel:        backupComponentTask,
		backupManagedByLabel:        "true",
		serviceGroupNameLabel:       clusterName,
		serviceGroupTypeLabel:       "clickhouse-sg",
		serviceTypeLabel:            "clickhouse",
		backupTaskTypeLabel:         taskType,
	}
}

func backupCronJobChildLabels(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	output[backupComponentLabel] = backupComponentTask
	output[backupTaskTypeLabel] = model.TaskTypeBackup
	return output
}

func backupTaskAnnotations(source model.BackupScope, target model.RestoreTarget, secretRef, backupPath, pathPrefix string) map[string]string {
	targetText := ""
	if target.Database != "" || target.Table != "" {
		targetText = target.Database + "." + target.Table
	}
	return map[string]string{
		backupPathAnnotation:       backupPath,
		backupPathPrefixAnnotation: pathPrefix,
		backupSecretAnnotation:     secretRef,
		backupSourceAnnotation:     source.Database + "." + source.Table,
		backupTargetAnnotation:     targetText,
	}
}

func backupScheduleResourceName(clusterName, scheduleName string) string {
	return clusterName + "-backup-" + scheduleName
}

func taskStatusFromJobStatus(job *batchv1.Job) string {
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			return model.TaskStatusSucceeded
		}
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			return model.TaskStatusFailed
		}
	}
	if job.Status.Active > 0 {
		return model.TaskStatusRunning
	}
	if job.Status.Failed > 0 {
		return model.TaskStatusFailed
	}
	if job.Status.Succeeded > 0 {
		return model.TaskStatusSucceeded
	}
	return model.TaskStatusPending
}

func taskMessageFromJob(job *batchv1.Job) string {
	for _, condition := range job.Status.Conditions {
		if condition.Message != "" {
			return condition.Message
		}
	}
	switch taskStatusFromJobStatus(job) {
	case model.TaskStatusSucceeded:
		return "task job completed"
	case model.TaskStatusRunning:
		return "task job is running"
	case model.TaskStatusFailed:
		return "task job failed"
	default:
		return "task job is pending"
	}
}

func backupSecretKeySelector(secretRef, key string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: secretRef},
		Key:                  key,
	}}
}

func emptyStringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func boolPointer(value bool) *bool {
	return &value
}

func int32Pointer(value int32) *int32 {
	return &value
}

func int64Pointer(value int64) *int64 {
	return &value
}

func pointerInt32Value(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}

func taskNotFoundError(namespace, name string) error {
	return &model.APIError{
		Status:  http.StatusNotFound,
		Code:    "BACKUP_TASK_NOT_FOUND",
		Message: "backup or restore task not found",
		Details: map[string]any{"namespace": namespace, "name": name},
	}
}

func backupScheduleNotFoundError(namespace, name string) error {
	return &model.APIError{
		Status:  http.StatusNotFound,
		Code:    "BACKUP_SCHEDULE_NOT_FOUND",
		Message: "backup schedule not found",
		Details: map[string]any{"namespace": namespace, "name": name},
	}
}

func backupJobScript() string {
	return `set -euo pipefail

log() {
  printf '[%s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    log "ERROR required environment variable ${name} is missing"
    exit 10
  fi
}

decrypt_secret() {
  local secret_file="${SECRET_MOUNT}/${CLICKHOUSE_ADMIN_PASSWORD_KEY}"
  if [[ ! -f "$secret_file" ]]; then
    log "ERROR admin password secret file is missing"
    exit 11
  fi
  local enc_key enc_iv
  enc_key="$(echo -n "${AES_SECRET_KEY}" | od -t x1 -An -v | tr -d ' \n')"
  enc_iv="$(head -c 16 "$secret_file" | od -t x1 -An -v | tr -d ' \n')"
  tail -c +17 "$secret_file" | openssl enc -d -aes-256-ctr -iv "$enc_iv" -K "$enc_key" 2>/dev/null
}

xml_escape() {
  local value="$1"
  value="${value//&/&amp;}"
  value="${value//</&lt;}"
  value="${value//>/&gt;}"
  printf "%s" "$value"
}

sql_string() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\'/\\\'}"
  printf "'%s'" "$value"
}

identifier() {
  local value="$1"
  case "$value" in
    ''|*[!A-Za-z0-9_]*|[0-9]*)
      log "ERROR invalid ClickHouse identifier"
      exit 12
      ;;
  esac
  printf '\x60%s\x60' "$value"
}

write_client_config() {
  local password="$1"
  umask 077
  cat > "$CLIENT_CONFIG" <<EOF
<config>
  <host>$(xml_escape "$CLICKHOUSE_HOST")</host>
  <port>$(xml_escape "$CLICKHOUSE_TCP_PORT")</port>
  <user>$(xml_escape "$CLICKHOUSE_ADMIN_USER")</user>
  <password>$(xml_escape "$password")</password>
</config>
EOF
}

run_sql() {
  clickhouse-client --config-file "$CLIENT_CONFIG" --multiquery
}

s3_url() {
  local endpoint="${S3_ENDPOINT%/}"
  local bucket="${S3_BUCKET#/}"
  bucket="${bucket%/}"
  local object="${BACKUP_PATH#/}"
  printf '%s/%s/%s' "$endpoint" "$bucket" "$object"
}

for name in OPERATION CLICKHOUSE_HOST CLICKHOUSE_TCP_PORT CLICKHOUSE_ADMIN_USER CLICKHOUSE_ADMIN_PASSWORD_KEY SECRET_MOUNT AES_SECRET_KEY SOURCE_DATABASE SOURCE_TABLE S3_ENDPOINT S3_BUCKET S3_ACCESS_KEY S3_SECRET_KEY; do
  require_env "$name"
done

if [[ -z "${BACKUP_PATH:-}" ]]; then
  require_env BACKUP_PATH_PREFIX
  BACKUP_PATH="${BACKUP_PATH_PREFIX%/}/$(date -u +%Y%m%dT%H%M%SZ)-${HOSTNAME}"
fi

CLIENT_CONFIG="$(mktemp /tmp/clickhouse-client.XXXXXX.xml)"
trap 'rm -f "$CLIENT_CONFIG"' EXIT
write_client_config "$(decrypt_secret)"

source_table="$(identifier "$SOURCE_DATABASE").$(identifier "$SOURCE_TABLE")"
s3="$(sql_string "$(s3_url)")"
access="$(sql_string "$S3_ACCESS_KEY")"
secret="$(sql_string "$S3_SECRET_KEY")"

case "$OPERATION" in
  backup)
    log "starting backup path=${BACKUP_PATH}"
    printf 'BACKUP TABLE %s ON CLUSTER %s TO S3(%s, %s, %s);\n' "$source_table" "upm_cluster" "$s3" "$access" "$secret" | run_sql
    log "backup completed path=${BACKUP_PATH}"
    ;;
  restore)
    require_env TARGET_DATABASE
    require_env TARGET_TABLE
    target_table="$(identifier "$TARGET_DATABASE").$(identifier "$TARGET_TABLE")"
    target_database="$(identifier "$TARGET_DATABASE")"
    target_database_string="$(sql_string "$TARGET_DATABASE")"
    target_table_string="$(sql_string "$TARGET_TABLE")"
    target_exists_message="$(sql_string "restore target table already exists")"
    log "starting restore path=${BACKUP_PATH} target=${TARGET_DATABASE}.${TARGET_TABLE}"
    {
      printf 'CREATE DATABASE IF NOT EXISTS %s ON CLUSTER %s;\n' "$target_database" "upm_cluster"
      printf 'SELECT throwIf(count() > 0, %s) FROM clusterAllReplicas(%s, system.tables) WHERE database = %s AND name = %s;\n' "$target_exists_message" "'upm_cluster'" "$target_database_string" "$target_table_string"
      printf 'CREATE TABLE %s ON CLUSTER %s AS %s ENGINE = MergeTree ORDER BY tuple();\n' "$target_table" "upm_cluster" "$source_table"
      printf 'RESTORE TABLE %s AS %s ON CLUSTER %s FROM S3(%s, %s, %s) SETTINGS allow_different_table_def=true;\n' "$source_table" "$target_table" "upm_cluster" "$s3" "$access" "$secret"
    } | run_sql
    log "restore completed path=${BACKUP_PATH} target=${TARGET_DATABASE}.${TARGET_TABLE}"
    ;;
  *)
    log "ERROR unsupported operation ${OPERATION}"
    exit 13
    ;;
esac
`
}
