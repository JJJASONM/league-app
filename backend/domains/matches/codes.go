package matches

// Schedule type codes for seasons.schedule_type.
// These values are stored in the database and drive generator selection in GenerateSchedule.
const (
	ScheduleTypeSingleRR = "single_rr"
	ScheduleTypeDoubleRR = "double_rr"
	ScheduleTypeSplit    = "split"
	ScheduleTypeCustom   = "custom"
	ScheduleTypeBlanket  = "blanket"
)
