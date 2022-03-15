package report

const (
	prometheusSelectorSuccessLabel = "success"
	prometheusSelectorFailedLabel  = "failed"
)

// ToPrometheusMetrics returns the report as Prometheus metrics served by the exporter.
func (b *Builder) ToPrometheusMetrics() error {
	b.finalize()
	// translate slice of Sections into a map structure
	nodeMap := make(map[string]map[string]map[string]struct {
		success []string
		failed  []string
	})
	for _, section := range b.Report.Sections {

		if nodeMap[section.File] == nil {
			nodeMap[section.File] = make(map[string]map[string]struct {
				success []string
				failed  []string
			})
		}
		if nodeMap[section.File][section.Group] == nil {
			nodeMap[section.File][section.Group] = make(map[string]struct {
				success []string
				failed  []string
			})
		}

		results := nodeMap[section.File][section.Group][section.Name]

		for _, s := range section.Results {
			results.success = append(results.success, s)
		}

		for _, s := range section.NoResults {
			results.failed = append(results.failed, s)
		}

		nodeMap[section.File][section.Group][section.Name] = results
	}

	// update metrics
	b.metrics.SetRulesTotal(float64(b.Report.TotalRules))
	b.metrics.SetRuleGroupsTotal(float64(b.Report.TotalGroups))

	for file, groups := range nodeMap {
		for group, rules := range groups {
			for rule, results := range rules {
				b.metrics.SetSelectorsTotal(file, group, rule, prometheusSelectorFailedLabel, float64(len(results.failed)))
				b.metrics.SetSelectorsTotal(file, group, rule, prometheusSelectorSuccessLabel, float64(len(results.success)))
			}
		}
	}
	return nil
}
