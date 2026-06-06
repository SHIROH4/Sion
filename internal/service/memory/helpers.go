package memory

import "fmt"
import "time"

func RelativeTime(ts int64) string {
	d := time.Since(time.Unix(ts, 0))
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("今天%02d:%02d", time.Unix(ts, 0).Hour(), time.Unix(ts, 0).Minute())
	case d < 48*time.Hour:
		return "昨天"
	default:
		return fmt.Sprintf("%d天前", int(d.Hours()/24))
	}
}
