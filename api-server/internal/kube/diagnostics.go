package kube

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/model"
)

func (s *Store) RunDiagnostics(ctx context.Context, namespace, name string, filter model.DiagnosticsFilter) (model.DiagnosticsReport, error) {
	filter = filter.WithDefaults()
	if err := filter.Validate(); err != nil {
		return model.DiagnosticsReport{}, &model.APIError{
			Status:  http.StatusBadRequest,
			Code:    "VALIDATION_ERROR",
			Message: err.Error(),
			Err:     err,
		}
	}

	cluster, err := s.GetCluster(ctx, namespace, name)
	if err != nil {
		return model.DiagnosticsReport{}, err
	}

	report := model.NewDiagnosticsReport(namespace, name, filter, s.diagnosticsThresholds)
	keeperPods, serverPods, err := s.listClusterPods(ctx, namespace, name)
	if err != nil {
		report.AddFinding("cluster", model.DiagnosticSeverityUnknown, "Cluster Pods could not be listed", map[string]any{
			"error": errString(err),
		}, "Check Kubernetes API connectivity and upm-api-server RBAC.", true)
		report.Finalize()
		return report, nil
	}

	s.addKeeperDiagnostics(ctx, &report, namespace, keeperPods)
	s.addStorageDiagnostics(ctx, &report, namespace, name, cluster.Topology)

	pod, ok := firstPod(serverPods)
	if !ok {
		for _, category := range []string{"replica", "replication_queue", "parts", "merges", "mutations", "write_client_stats", "write_quality"} {
			report.AddFinding(category, model.DiagnosticSeverityCritical, "No ClickHouse Server Pod is available for diagnostics", map[string]any{
				"serverPods": podNames(serverPods),
			}, "Restore ClickHouse Server Pod readiness before running SQL-based diagnostics.", true)
		}
		report.Finalize()
		return report, nil
	}

	s.addReplicaDiagnostics(ctx, &report, namespace, pod.Name, filter)
	s.addReplicationQueueDiagnostics(ctx, &report, namespace, pod.Name, filter)
	s.addPartDiagnostics(ctx, &report, namespace, pod.Name, filter)
	s.addMergeDiagnostics(ctx, &report, namespace, pod.Name, filter)
	s.addMutationDiagnostics(ctx, &report, namespace, pod.Name, filter)
	s.addWriteClientStatsDiagnostics(ctx, &report, namespace, pod.Name, filter)
	s.addWriteQualityDiagnostics(ctx, &report, namespace, pod.Name, filter)
	report.Finalize()
	return report, nil
}

func (s *Store) addReplicaDiagnostics(ctx context.Context, report *model.DiagnosticsReport, namespace, podName string, filter model.DiagnosticsFilter) {
	query := fmt.Sprintf(
		"SELECT hostName(), database, table, is_readonly, is_session_expired, absolute_delay, queue_size, future_parts FROM clusterAllReplicas('%s', system.replicas)%s ORDER BY absolute_delay DESC, queue_size DESC, hostName() LIMIT %d FORMAT TSV",
		clickHouseClusterName,
		diagnosticsWhere(filter, nil),
		filter.Limit,
	)
	rows, err := s.diagnosticQueryRows(ctx, namespace, podName, query)
	if err != nil {
		addDiagnosticQueryFailure(report, "replica", "Replica diagnostics query failed", err, query, "Check ClickHouse SQL reachability and system.replicas availability.")
		return
	}
	if len(rows) == 0 {
		report.AddFinding("replica", model.DiagnosticSeverityInfo, "No replicated tables matched the diagnostics filter", map[string]any{
			"samplePod": podName,
			"filters":   filter,
		}, "No action required when the filter intentionally excludes replicated tables.", false)
		return
	}

	thresholds := report.Thresholds
	replicas := make([]map[string]any, 0, len(rows))
	issues := []map[string]any{}
	severity := model.DiagnosticSeverityInfo
	for _, row := range rows {
		if len(row) < 8 {
			severity = maxDiagnosticSeverity(severity, model.DiagnosticSeverityUnknown)
			issues = append(issues, map[string]any{"row": row, "reason": "unexpected column count"})
			continue
		}
		readOnly := parseInt(row[3])
		sessionExpired := parseInt(row[4])
		delay := parseInt(row[5])
		queueSize := parseInt(row[6])
		futureParts := parseInt(row[7])
		item := map[string]any{
			"host":             row[0],
			"database":         row[1],
			"table":            row[2],
			"isReadonly":       readOnly,
			"isSessionExpired": sessionExpired,
			"absoluteDelay":    delay,
			"queueSize":        queueSize,
			"futureParts":      futureParts,
		}
		replicas = append(replicas, item)
		itemSeverity := model.DiagnosticSeverityInfo
		reasons := []string{}
		if readOnly != 0 {
			itemSeverity = maxDiagnosticSeverity(itemSeverity, model.DiagnosticSeverityCritical)
			reasons = append(reasons, "readonly")
		}
		if sessionExpired != 0 {
			itemSeverity = maxDiagnosticSeverity(itemSeverity, model.DiagnosticSeverityCritical)
			reasons = append(reasons, "session_expired")
		}
		if delay >= thresholds.ReplicaDelayCriticalSeconds {
			itemSeverity = maxDiagnosticSeverity(itemSeverity, model.DiagnosticSeverityCritical)
			reasons = append(reasons, "absolute_delay_critical")
		} else if delay >= thresholds.ReplicaDelayWarnSeconds {
			itemSeverity = maxDiagnosticSeverity(itemSeverity, model.DiagnosticSeverityWarn)
			reasons = append(reasons, "absolute_delay_warn")
		}
		if queueSize >= thresholds.ReplicationQueueCritical {
			itemSeverity = maxDiagnosticSeverity(itemSeverity, model.DiagnosticSeverityCritical)
			reasons = append(reasons, "queue_size_critical")
		} else if queueSize >= thresholds.ReplicationQueueWarn {
			itemSeverity = maxDiagnosticSeverity(itemSeverity, model.DiagnosticSeverityWarn)
			reasons = append(reasons, "queue_size_warn")
		}
		if itemSeverity != model.DiagnosticSeverityInfo {
			issue := copyStringAnyMap(item)
			issue["reasons"] = reasons
			issues = append(issues, issue)
			severity = maxDiagnosticSeverity(severity, itemSeverity)
		}
	}
	title := "Replicas are writable and caught up"
	recommendation := "No action required."
	if severity != model.DiagnosticSeverityInfo {
		title = "One or more replicas need DBA review"
		recommendation = "Inspect Keeper connectivity, replica delay, and replication queue pressure before taking remediation actions."
	}
	report.AddFinding("replica", severity, title, map[string]any{
		"samplePod": podName,
		"replicas":  replicas,
		"issues":    issues,
	}, recommendation, severity != model.DiagnosticSeverityInfo)
}

func (s *Store) addReplicationQueueDiagnostics(ctx context.Context, report *model.DiagnosticsReport, namespace, podName string, filter model.DiagnosticsFilter) {
	query := fmt.Sprintf(
		"SELECT database, table, count() AS queue_size, max(dateDiff('second', create_time, now())) AS oldest_age_seconds, anyLast(last_exception) AS last_exception FROM clusterAllReplicas('%s', system.replication_queue)%s GROUP BY database, table ORDER BY queue_size DESC, oldest_age_seconds DESC LIMIT %d FORMAT TSV",
		clickHouseClusterName,
		diagnosticsWhere(filter, nil),
		filter.Limit,
	)
	rows, err := s.diagnosticQueryRows(ctx, namespace, podName, query)
	if err != nil {
		addDiagnosticQueryFailure(report, "replication_queue", "Replication queue diagnostics query failed", err, query, "Check system.replication_queue availability and ClickHouse SQL reachability.")
		return
	}
	if len(rows) == 0 {
		report.AddFinding("replication_queue", model.DiagnosticSeverityInfo, "Replication queue is empty", map[string]any{
			"samplePod": podName,
		}, "No action required.", false)
		return
	}

	thresholds := report.Thresholds
	queues := make([]map[string]any, 0, len(rows))
	issues := []map[string]any{}
	severity := model.DiagnosticSeverityInfo
	for _, row := range rows {
		if len(row) < 5 {
			severity = maxDiagnosticSeverity(severity, model.DiagnosticSeverityUnknown)
			issues = append(issues, map[string]any{"row": row, "reason": "unexpected column count"})
			continue
		}
		queueSize := parseInt(row[2])
		oldestAge := parseInt(row[3])
		lastException := row[4]
		item := map[string]any{
			"database":         row[0],
			"table":            row[1],
			"queueSize":        queueSize,
			"oldestAgeSeconds": oldestAge,
			"lastException":    trimEvidence(lastException),
		}
		queues = append(queues, item)
		itemSeverity := model.DiagnosticSeverityInfo
		reasons := []string{}
		if queueSize >= thresholds.ReplicationQueueCritical {
			itemSeverity = maxDiagnosticSeverity(itemSeverity, model.DiagnosticSeverityCritical)
			reasons = append(reasons, "queue_size_critical")
		} else if queueSize >= thresholds.ReplicationQueueWarn {
			itemSeverity = maxDiagnosticSeverity(itemSeverity, model.DiagnosticSeverityWarn)
			reasons = append(reasons, "queue_size_warn")
		}
		if strings.TrimSpace(lastException) != "" {
			itemSeverity = maxDiagnosticSeverity(itemSeverity, model.DiagnosticSeverityCritical)
			reasons = append(reasons, "last_exception")
		}
		if itemSeverity != model.DiagnosticSeverityInfo {
			issue := copyStringAnyMap(item)
			issue["reasons"] = reasons
			issues = append(issues, issue)
			severity = maxDiagnosticSeverity(severity, itemSeverity)
		}
	}
	title := "Replication queue has pending tasks"
	recommendation := "Review queue size, oldest age, and last exceptions before scheduling any repair."
	if severity == model.DiagnosticSeverityInfo {
		title = "Replication queue has tasks within configured thresholds"
		recommendation = "No action required unless queue growth continues."
	}
	report.AddFinding("replication_queue", severity, title, map[string]any{
		"samplePod": podName,
		"queues":    queues,
		"issues":    issues,
	}, recommendation, severity != model.DiagnosticSeverityInfo)
}

func (s *Store) addPartDiagnostics(ctx context.Context, report *model.DiagnosticsReport, namespace, podName string, filter model.DiagnosticsFilter) {
	tableQuery := fmt.Sprintf(
		"SELECT database, table, countIf(active) AS active_parts, countIf(NOT active) AS inactive_parts, sumIf(rows, active) AS active_rows, sumIf(bytes_on_disk, active) AS active_bytes FROM clusterAllReplicas('%s', system.parts)%s GROUP BY database, table ORDER BY active_parts DESC, active_bytes DESC LIMIT %d FORMAT TSV",
		clickHouseClusterName,
		diagnosticsWhere(filter, nil),
		filter.Limit,
	)
	tableRows, err := s.diagnosticQueryRows(ctx, namespace, podName, tableQuery)
	if err != nil {
		addDiagnosticQueryFailure(report, "parts", "Parts table diagnostics query failed", err, tableQuery, "Check system.parts availability and query filters.")
		return
	}
	partitionQuery := fmt.Sprintf(
		"SELECT database, table, partition, count() AS active_parts, sum(rows) AS rows, sum(bytes_on_disk) AS bytes FROM clusterAllReplicas('%s', system.parts)%s GROUP BY database, table, partition ORDER BY active_parts DESC, bytes DESC LIMIT %d FORMAT TSV",
		clickHouseClusterName,
		diagnosticsWhere(filter, []string{"active"}),
		filter.Limit,
	)
	partitionRows, err := s.diagnosticQueryRows(ctx, namespace, podName, partitionQuery)
	if err != nil {
		addDiagnosticQueryFailure(report, "parts", "Partition diagnostics query failed", err, partitionQuery, "Check system.parts availability and query filters.")
		return
	}

	thresholds := report.Thresholds
	tables := make([]map[string]any, 0, len(tableRows))
	partitions := make([]map[string]any, 0, len(partitionRows))
	issues := []map[string]any{}
	severity := model.DiagnosticSeverityInfo
	for _, row := range tableRows {
		if len(row) < 6 {
			severity = maxDiagnosticSeverity(severity, model.DiagnosticSeverityUnknown)
			issues = append(issues, map[string]any{"row": row, "reason": "unexpected table column count"})
			continue
		}
		inactiveParts := parseInt(row[3])
		item := map[string]any{
			"database":      row[0],
			"table":         row[1],
			"activeParts":   parseInt(row[2]),
			"inactiveParts": inactiveParts,
			"activeRows":    parseInt64(row[4]),
			"activeBytes":   parseInt64(row[5]),
		}
		tables = append(tables, item)
		if inactiveParts >= thresholds.InactivePartsWarn {
			issue := copyStringAnyMap(item)
			issue["reason"] = "inactive_parts_warn"
			issues = append(issues, issue)
			severity = maxDiagnosticSeverity(severity, model.DiagnosticSeverityWarn)
		}
	}
	for _, row := range partitionRows {
		if len(row) < 6 {
			severity = maxDiagnosticSeverity(severity, model.DiagnosticSeverityUnknown)
			issues = append(issues, map[string]any{"row": row, "reason": "unexpected partition column count"})
			continue
		}
		activeParts := parseInt(row[3])
		item := map[string]any{
			"database":    row[0],
			"table":       row[1],
			"partition":   row[2],
			"activeParts": activeParts,
			"rows":        parseInt64(row[4]),
			"bytes":       parseInt64(row[5]),
		}
		partitions = append(partitions, item)
		if activeParts >= thresholds.ActivePartsPerPartitionWarn {
			issue := copyStringAnyMap(item)
			issue["reason"] = "active_parts_per_partition_warn"
			issues = append(issues, issue)
			severity = maxDiagnosticSeverity(severity, model.DiagnosticSeverityWarn)
		}
	}
	title := "Parts and partitions are within configured thresholds"
	recommendation := "No action required."
	if severity != model.DiagnosticSeverityInfo {
		title = "Parts or partitions need compaction review"
		recommendation = "Review active and inactive part counts before considering OPTIMIZE or TTL-related actions."
	}
	report.AddFinding("parts", severity, title, map[string]any{
		"samplePod":  podName,
		"tables":     tables,
		"partitions": partitions,
		"issues":     issues,
	}, recommendation, severity != model.DiagnosticSeverityInfo)
}

func (s *Store) addMergeDiagnostics(ctx context.Context, report *model.DiagnosticsReport, namespace, podName string, filter model.DiagnosticsFilter) {
	query := fmt.Sprintf(
		"SELECT database, table, elapsed, progress, num_parts, result_part_name FROM clusterAllReplicas('%s', system.merges)%s ORDER BY elapsed DESC LIMIT %d FORMAT TSV",
		clickHouseClusterName,
		diagnosticsWhere(filter, nil),
		filter.Limit,
	)
	rows, err := s.diagnosticQueryRows(ctx, namespace, podName, query)
	if err != nil {
		addDiagnosticQueryFailure(report, "merges", "Merge diagnostics query failed", err, query, "Check system.merges availability and query filters.")
		return
	}
	if len(rows) == 0 {
		report.AddFinding("merges", model.DiagnosticSeverityInfo, "No running merges matched the diagnostics filter", map[string]any{
			"samplePod": podName,
		}, "No action required.", false)
		return
	}
	merges := make([]map[string]any, 0, len(rows))
	issues := []map[string]any{}
	severity := model.DiagnosticSeverityInfo
	for _, row := range rows {
		if len(row) < 6 {
			severity = maxDiagnosticSeverity(severity, model.DiagnosticSeverityUnknown)
			issues = append(issues, map[string]any{"row": row, "reason": "unexpected column count"})
			continue
		}
		elapsed := parseFloat(row[2])
		item := map[string]any{
			"database":       row[0],
			"table":          row[1],
			"elapsedSeconds": elapsed,
			"progress":       parseFloat(row[3]),
			"numParts":       parseInt(row[4]),
			"resultPartName": row[5],
		}
		merges = append(merges, item)
		if int(elapsed) >= report.Thresholds.MergeElapsedWarnSeconds {
			issue := copyStringAnyMap(item)
			issue["reason"] = "merge_elapsed_warn"
			issues = append(issues, issue)
			severity = maxDiagnosticSeverity(severity, model.DiagnosticSeverityWarn)
		}
	}
	title := "Running merges are within configured thresholds"
	recommendation := "No action required."
	if severity != model.DiagnosticSeverityInfo {
		title = "Long-running merges need DBA review"
		recommendation = "Review merge backlog and resource pressure; do not kill or optimize automatically from this diagnostics API."
	}
	report.AddFinding("merges", severity, title, map[string]any{
		"samplePod": podName,
		"merges":    merges,
		"issues":    issues,
	}, recommendation, severity != model.DiagnosticSeverityInfo)
}

func (s *Store) addMutationDiagnostics(ctx context.Context, report *model.DiagnosticsReport, namespace, podName string, filter model.DiagnosticsFilter) {
	query := fmt.Sprintf(
		"SELECT database, table, mutation_id, command, is_done, dateDiff('second', create_time, now()) AS age_seconds, latest_failed_part, latest_fail_time, latest_fail_reason FROM clusterAllReplicas('%s', system.mutations)%s ORDER BY create_time DESC LIMIT %d FORMAT TSV",
		clickHouseClusterName,
		diagnosticsWhere(filter, nil),
		filter.Limit,
	)
	rows, err := s.diagnosticQueryRows(ctx, namespace, podName, query)
	if err != nil {
		addDiagnosticQueryFailure(report, "mutations", "Mutation diagnostics query failed", err, query, "Check system.mutations availability and query filters.")
		return
	}
	if len(rows) == 0 {
		report.AddFinding("mutations", model.DiagnosticSeverityInfo, "No mutations matched the diagnostics filter", map[string]any{
			"samplePod": podName,
		}, "No action required.", false)
		return
	}
	mutations := make([]map[string]any, 0, len(rows))
	issues := []map[string]any{}
	severity := model.DiagnosticSeverityInfo
	for _, row := range rows {
		if len(row) < 9 {
			severity = maxDiagnosticSeverity(severity, model.DiagnosticSeverityUnknown)
			issues = append(issues, map[string]any{"row": row, "reason": "unexpected column count"})
			continue
		}
		isDone := parseInt(row[4])
		age := parseInt(row[5])
		failedReason := row[8]
		item := map[string]any{
			"database":         row[0],
			"table":            row[1],
			"mutationId":       row[2],
			"command":          trimEvidence(row[3]),
			"isDone":           isDone,
			"ageSeconds":       age,
			"latestFailedPart": row[6],
			"latestFailTime":   row[7],
			"latestFailReason": trimEvidence(failedReason),
		}
		mutations = append(mutations, item)
		itemSeverity := model.DiagnosticSeverityInfo
		reasons := []string{}
		if strings.TrimSpace(failedReason) != "" {
			itemSeverity = maxDiagnosticSeverity(itemSeverity, model.DiagnosticSeverityCritical)
			reasons = append(reasons, "latest_fail_reason")
		}
		if isDone == 0 && age >= report.Thresholds.MutationAgeWarnSeconds {
			itemSeverity = maxDiagnosticSeverity(itemSeverity, model.DiagnosticSeverityWarn)
			reasons = append(reasons, "unfinished_mutation_age_warn")
		}
		if itemSeverity != model.DiagnosticSeverityInfo {
			issue := copyStringAnyMap(item)
			issue["reasons"] = reasons
			issues = append(issues, issue)
			severity = maxDiagnosticSeverity(severity, itemSeverity)
		}
	}
	title := "Mutations are complete or within configured thresholds"
	recommendation := "No action required."
	if severity != model.DiagnosticSeverityInfo {
		title = "One or more mutations need DBA review"
		recommendation = "Inspect failed or unfinished mutations manually; this diagnostics API does not kill mutations."
	}
	report.AddFinding("mutations", severity, title, map[string]any{
		"samplePod": podName,
		"mutations": mutations,
		"issues":    issues,
	}, recommendation, severity != model.DiagnosticSeverityInfo)
}

func (s *Store) addKeeperDiagnostics(ctx context.Context, report *model.DiagnosticsReport, namespace string, keeperPods []corev1.Pod) {
	if len(keeperPods) == 0 {
		report.AddFinding("keeper", model.DiagnosticSeverityCritical, "No Keeper Pods are available", nil, "Restore Keeper Pods before trusting ClickHouse replication diagnostics.", true)
		return
	}
	roles := map[string]string{}
	failures := map[string]string{}
	leaders := 0
	for _, pod := range keeperPods {
		stdout, stderr, err := s.execPod(ctx, namespace, pod.Name, "clickhouse-keeper", []string{"bash", "-lc", "exec 3<>/dev/tcp/127.0.0.1/9181; printf mntr >&3; timeout 2 cat <&3"}, "")
		role := parseKeeperRole(stdout)
		roles[pod.Name] = role
		if role == "leader" {
			leaders++
		}
		if err != nil || (role != "leader" && role != "follower") {
			failures[pod.Name] = trimEvidence(stderr + " " + errString(err))
		}
	}
	severity := model.DiagnosticSeverityInfo
	title := "Keeper quorum has one leader and followers"
	recommendation := "No action required."
	if leaders != 1 || len(failures) > 0 {
		severity = model.DiagnosticSeverityCritical
		title = "Keeper leader/follower state is invalid"
		recommendation = "Check Keeper Pod health, network reachability, and Keeper logs before performing ClickHouse replica operations."
	}
	report.AddFinding("keeper", severity, title, map[string]any{
		"roles":    roles,
		"leaders":  leaders,
		"failures": failures,
	}, recommendation, severity != model.DiagnosticSeverityInfo)
}

func (s *Store) addStorageDiagnostics(ctx context.Context, report *model.DiagnosticsReport, namespace, name string, topology model.Topology) {
	pvcs, err := s.core.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		report.AddFinding("storage", model.DiagnosticSeverityUnknown, "PVCs could not be listed", map[string]any{
			"error": errString(err),
		}, "Check Kubernetes API connectivity and upm-api-server RBAC.", true)
		return
	}
	expected := topology.KeeperReplicas + topology.Shards*topology.ReplicasPerShard
	items := []map[string]any{}
	notBound := []string{}
	for i := range pvcs.Items {
		pvc := pvcs.Items[i]
		if !storageResourceNameMatches(pvc.Name, name) {
			continue
		}
		capacity := ""
		if value, exists := pvc.Status.Capacity[corev1.ResourceStorage]; exists {
			capacity = value.String()
		}
		storageClass := ""
		if pvc.Spec.StorageClassName != nil {
			storageClass = *pvc.Spec.StorageClassName
		}
		items = append(items, map[string]any{
			"name":         pvc.Name,
			"phase":        string(pvc.Status.Phase),
			"capacity":     capacity,
			"storageClass": storageClass,
			"volumeName":   pvc.Spec.VolumeName,
		})
		if pvc.Status.Phase != corev1.ClaimBound {
			notBound = append(notBound, pvc.Name)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["name"].(string) < items[j]["name"].(string) })
	severity := model.DiagnosticSeverityInfo
	title := "PVCs are bound and capacity metadata is visible"
	recommendation := "No action required."
	if len(items) != expected || len(notBound) > 0 {
		severity = model.DiagnosticSeverityCritical
		title = "PVC count or binding state does not match expected topology"
		recommendation = "Check PVC binding, StorageClass behavior, and node-local storage before writing more data."
	}
	report.AddFinding("storage", severity, title, map[string]any{
		"expectedPVCs": expected,
		"actualPVCs":   len(items),
		"notBoundPVCs": notBound,
		"pvcs":         items,
	}, recommendation, severity != model.DiagnosticSeverityInfo)
}

func (s *Store) addWriteClientStatsDiagnostics(ctx context.Context, report *model.DiagnosticsReport, namespace, podName string, filter model.DiagnosticsFilter) {
	exists, err := s.diagnosticScalar(ctx, namespace, podName, "EXISTS TABLE system.query_log FORMAT TSV")
	if err != nil || strings.TrimSpace(exists) != "1" {
		report.AddFinding("write_client_stats", model.DiagnosticSeverityUnknown, "system.query_log is not available", map[string]any{
			"exists": strings.TrimSpace(exists),
			"error":  errString(err),
		}, "Enable ClickHouse query_log to collect write client statistics by user, client IP, and target table.", false)
		return
	}
	conditions := []string{"event_time >= now() - INTERVAL 24 HOUR", "type = 'QueryFinish'", "query_kind = 'Insert'"}
	if filter.Database != "" && filter.Table != "" {
		conditions = append(conditions, fmt.Sprintf("has(tables, %s)", clickHouseStringLiteral(filter.Database+"."+filter.Table)))
	}
	query := fmt.Sprintf(
		"SELECT initial_user, addressToString(initial_address), arrayStringConcat(tables, ','), count(), sum(written_rows), sum(written_bytes) FROM clusterAllReplicas('%s', system.query_log) WHERE %s GROUP BY initial_user, initial_address, tables ORDER BY count() DESC LIMIT %d FORMAT TSV",
		clickHouseClusterName,
		strings.Join(conditions, " AND "),
		filter.Limit,
	)
	rows, err := s.diagnosticQueryRows(ctx, namespace, podName, query)
	if err != nil {
		addDiagnosticQueryFailure(report, "write_client_stats", "Write client statistics query failed", err, query, "Check query_log schema and ClickHouse query log settings.")
		return
	}
	if len(rows) == 0 {
		report.AddFinding("write_client_stats", model.DiagnosticSeverityInfo, "No recent INSERT query_log rows matched the diagnostics filter", map[string]any{
			"window": "24h",
		}, "No action required, or enable workload before checking write client statistics.", false)
		return
	}
	clients := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if len(row) < 6 {
			continue
		}
		clients = append(clients, map[string]any{
			"user":         row[0],
			"clientIP":     row[1],
			"targetTables": row[2],
			"queries":      parseInt(row[3]),
			"writtenRows":  parseInt64(row[4]),
			"writtenBytes": parseInt64(row[5]),
		})
	}
	report.AddFinding("write_client_stats", model.DiagnosticSeverityInfo, "Recent write client statistics are available", map[string]any{
		"window":  "24h",
		"clients": clients,
	}, "Use this read-only summary for DBA investigation; persistent write analytics is future scope.", false)
}

func (s *Store) addWriteQualityDiagnostics(ctx context.Context, report *model.DiagnosticsReport, namespace, podName string, filter model.DiagnosticsFilter) {
	if filter.Database == "" || filter.Table == "" {
		report.AddFinding("write_quality", model.DiagnosticSeverityUnknown, "Write quality row-count criteria were not provided", map[string]any{
			"required": []string{"database", "table"},
			"optional": []string{"partition", "timeColumn", "startTime", "endTime", "expectedRows"},
		}, "Provide database and table, plus optional partition/time filters and expectedRows, to run a read-only row-count validation.", false)
		return
	}
	conditions := []string{}
	if filter.Partition != "" {
		conditions = append(conditions, fmt.Sprintf("_partition_id = %s", clickHouseStringLiteral(filter.Partition)))
	}
	if filter.StartTime != "" {
		conditions = append(conditions, fmt.Sprintf("%s >= parseDateTimeBestEffort(%s)", clickHouseIdentifier(filter.TimeColumn), clickHouseStringLiteral(filter.StartTime)))
	}
	if filter.EndTime != "" {
		conditions = append(conditions, fmt.Sprintf("%s < parseDateTimeBestEffort(%s)", clickHouseIdentifier(filter.TimeColumn), clickHouseStringLiteral(filter.EndTime)))
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	query := fmt.Sprintf("SELECT count() FROM %s.%s%s FORMAT TSV", clickHouseIdentifier(filter.Database), clickHouseIdentifier(filter.Table), where)
	value, err := s.diagnosticScalar(ctx, namespace, podName, query)
	if err != nil {
		addDiagnosticQueryFailure(report, "write_quality", "Read-only row-count validation query failed", err, query, "Check database/table existence and optional partition/time filter correctness.")
		return
	}
	actualRows := parseInt64(strings.TrimSpace(value))
	severity := model.DiagnosticSeverityInfo
	title := "Read-only row-count validation completed"
	recommendation := "No action required unless expectedRows is provided and does not match."
	evidence := map[string]any{
		"database":   filter.Database,
		"table":      filter.Table,
		"partition":  filter.Partition,
		"timeColumn": filter.TimeColumn,
		"startTime":  filter.StartTime,
		"endTime":    filter.EndTime,
		"actualRows": actualRows,
	}
	if filter.ExpectedRows == nil {
		evidence["expectedRowsProvided"] = false
	} else {
		evidence["expectedRowsProvided"] = true
		evidence["expectedRows"] = *filter.ExpectedRows
		if actualRows != *filter.ExpectedRows {
			severity = model.DiagnosticSeverityWarn
			title = "Read-only row-count validation does not match expectedRows"
			recommendation = "Review ingest window, partition filter, and application write path before taking corrective action."
		}
	}
	report.AddFinding("write_quality", severity, title, evidence, recommendation, severity != model.DiagnosticSeverityInfo)
}

func (s *Store) diagnosticQueryRows(ctx context.Context, namespace, podName, query string) ([][]string, error) {
	stdout, stderr, err := s.clickHouseQuery(ctx, namespace, podName, query)
	if err != nil {
		return nil, fmt.Errorf("%s %s", trimEvidence(stderr), errString(err))
	}
	return parseTSV(stdout), nil
}

func (s *Store) diagnosticScalar(ctx context.Context, namespace, podName, query string) (string, error) {
	stdout, stderr, err := s.clickHouseQuery(ctx, namespace, podName, query)
	if err != nil {
		return "", fmt.Errorf("%s %s", trimEvidence(stderr), errString(err))
	}
	rows := parseTSV(stdout)
	if len(rows) == 0 || len(rows[0]) == 0 {
		return "", nil
	}
	return rows[0][0], nil
}

func diagnosticsWhere(filter model.DiagnosticsFilter, extra []string) string {
	conditions := append([]string{}, extra...)
	if filter.Database != "" {
		conditions = append(conditions, "database = "+clickHouseStringLiteral(filter.Database))
	}
	if filter.Table != "" {
		conditions = append(conditions, "table = "+clickHouseStringLiteral(filter.Table))
	}
	if len(conditions) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(conditions, " AND ")
}

func addDiagnosticQueryFailure(report *model.DiagnosticsReport, category, title string, err error, query, recommendation string) {
	report.AddFinding(category, model.DiagnosticSeverityUnknown, title, map[string]any{
		"error": errString(err),
		"query": trimEvidence(query),
	}, recommendation, true)
}

func clickHouseStringLiteral(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "'", "\\'")
	return "'" + value + "'"
}

func clickHouseIdentifier(value string) string {
	value = strings.ReplaceAll(value, "`", "``")
	return "`" + value + "`"
}

func parseInt(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return parsed
}

func parseInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}

func parseFloat(value string) float64 {
	parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed
}

func maxDiagnosticSeverity(current, next string) string {
	if diagnosticSeverityRank(next) > diagnosticSeverityRank(current) {
		return next
	}
	return current
}

func diagnosticSeverityRank(severity string) int {
	switch severity {
	case model.DiagnosticSeverityCritical:
		return 4
	case model.DiagnosticSeverityWarn:
		return 3
	case model.DiagnosticSeverityUnknown:
		return 2
	case model.DiagnosticSeverityInfo:
		return 1
	default:
		return 0
	}
}

func copyStringAnyMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
