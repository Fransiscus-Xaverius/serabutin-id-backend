package dto

type CreateBookingRequest struct {
	ServiceID    uint   `json:"service_id" validate:"required"`
	ScheduleDate string `json:"schedule_date" validate:"required"`
	ScheduleTime string `json:"schedule_time" validate:"required"`
	Address      string `json:"address" validate:"required,max=255"`
	Notes        string `json:"notes,omitempty" validate:"omitempty,max=500"`
	AddOnIDs     []uint `json:"add_on_ids,omitempty"`
	// PaymentMethod is the channel slug of an enabled prepayment method
	// (see GET /api/v1/booking-config). The enabled set is a runtime
	// configuration, so the usecase validates it.
	PaymentMethod string `json:"payment_method" validate:"required,max=30"`
}

type AcceptBookingRequest struct {
	ScheduleDate string `json:"schedule_date" validate:"omitempty,datetime=2006-01-02"`
	ScheduleTime string `json:"schedule_time" validate:"omitempty,len=5"`
	Address      string `json:"address" validate:"omitempty,max=255"`
}

type CancelBookingRequest struct {
	Reason string `json:"reason" validate:"required,oneof=changed_mind found_other schedule_conflict no_response other"`
	Detail string `json:"detail,omitempty" validate:"omitempty,max=500"`
}

type CancellationResponse struct {
	ID              uint   `json:"id"`
	BookingID       uint   `json:"booking_id"`
	CancelledBy     uint   `json:"cancelled_by"`
	CancelledByRole string `json:"cancelled_by_role"`
	Reason          string `json:"reason"`
	Detail          string `json:"detail,omitempty"`
	FeeCharged      int64  `json:"fee_charged"`
	PolicyVersion   string `json:"policy_version"`
	CreatedAt       string `json:"created_at"`
}

type BookingResponse struct {
	ID             uint                   `json:"id"`
	Customer       *BookingUserInfo       `json:"customer,omitempty"`
	Service        *BookingServiceInfo    `json:"service,omitempty"`
	Provider       *BookingUserInfo       `json:"provider,omitempty"`
	ScheduleDate   *string                `json:"schedule_date"`
	ScheduleTime   *string                `json:"schedule_time"`
	Address        *string                `json:"address"`
	Notes          string                 `json:"notes,omitempty"`
	AddOns         []BookingAddOnResponse `json:"add_ons"`
	SubTotal       int64                  `json:"sub_total"`
	PlatformFee    int64                  `json:"platform_fee"`
	Total          int64                  `json:"total"`
	PaymentMethod  string                 `json:"payment_method"`
	Status         string                 `json:"status"`
	Payment        *PaymentResponse       `json:"payment,omitempty"`
	CancellationID *uint                  `json:"cancellation_id,omitempty"`
	CancelledBy    *uint                  `json:"cancelled_by,omitempty"`
	CancelledAt    *string                `json:"cancelled_at,omitempty"`
	Cancellation   *CancellationResponse  `json:"cancellation,omitempty"`
	CreatedAt      string                 `json:"created_at"`
}

type BookingUserInfo struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

type BookingServiceInfo struct {
	ID          uint   `json:"id"`
	Title       string `json:"title"`
	ServiceType string `json:"service_type,omitempty"`
}

type BookingAddOnResponse struct {
	Name  string `json:"name"`
	Price int64  `json:"price"`
}

type BookingListResponse struct {
	Data       []BookingResponse `json:"data"`
	Pagination PaginationMeta    `json:"pagination"`
}
