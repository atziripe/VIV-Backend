package dto

type TrainingPlan interface {
	PlanType() string
	GuidanceLevel() string
	StartDate() string
	EndDate() string
}

func (p FullGuidancePlan) PlanType() string      { return p.Meta.PlanType }
func (p FullGuidancePlan) GuidanceLevel() string { return p.Meta.GuidanceLevel }
func (p FullGuidancePlan) StartDate() string     { return p.WeekStart }
func (p FullGuidancePlan) EndDate() string       { return p.WeekEnd }

func (p GuidedSupportPlan) PlanType() string      { return p.Meta.PlanType }
func (p GuidedSupportPlan) GuidanceLevel() string { return p.Meta.GuidanceLevel }
func (p GuidedSupportPlan) StartDate() string     { return p.WeekStart }
func (p GuidedSupportPlan) EndDate() string       { return p.WeekEnd }

func (p LightGuidancePlan) PlanType() string      { return p.Meta.PlanType }
func (p LightGuidancePlan) GuidanceLevel() string { return p.Meta.GuidanceLevel }
func (p LightGuidancePlan) StartDate() string     { return p.WeekStart }
func (p LightGuidancePlan) EndDate() string       { return p.WeekEnd }
