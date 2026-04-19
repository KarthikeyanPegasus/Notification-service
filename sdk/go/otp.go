package notification

import "context"

type OTPService struct {
	client *Client
}

func (s *OTPService) Send(ctx context.Context, req *OTPSendRequest) (*OTPSendResponse, error) {
	var out OTPSendResponse
	if err := s.client.do(ctx, "POST", "/otp/send", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *OTPService) Verify(ctx context.Context, req *OTPVerifyRequest) (*OTPVerifyResponse, error) {
	var out OTPVerifyResponse
	if err := s.client.do(ctx, "POST", "/otp/verify", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
