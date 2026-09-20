package geo

import (
	"errors"
	"fmt"
	"testing"
)

func assertUnknownCountry(t *testing.T, err error, country string) {
	t.Helper()
	var got *UnknownCountryError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *UnknownCountryError", err, err)
	}
	if got.Country != country {
		t.Fatalf("UnknownCountryError.Country = %q, want %q", got.Country, country)
	}
	t.Logf("errors.As → *UnknownCountryError Country=%q err=%v", got.Country, err)
}

func assertInvalidExtra(t *testing.T, err error, extra int) {
	t.Helper()
	var got *InvalidExtraLanguageCountError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *InvalidExtraLanguageCountError", err, err)
	}
	if got.Extra != extra {
		t.Fatalf("InvalidExtraLanguageCountError.Extra = %d, want %d", got.Extra, extra)
	}
	t.Logf("errors.As → *InvalidExtraLanguageCountError Extra=%d err=%v", got.Extra, err)
}

func assertExtraExceedsPool(t *testing.T, err error, extra, available int) {
	t.Helper()
	var got *ExtraLanguageCountExceedsPoolError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *ExtraLanguageCountExceedsPoolError", err, err)
	}
	if got.Extra != extra || got.Available != available {
		t.Fatalf("Extra=%d Available=%d, want Extra=%d Available=%d", got.Extra, got.Available, extra, available)
	}
	t.Logf("errors.As → *ExtraLanguageCountExceedsPoolError Extra=%d Available=%d err=%v", got.Extra, got.Available, err)
}

func TestUnknownCountryError_未知码文案(t *testing.T) {
	err := &UnknownCountryError{Country: "XX"}
	assertUnknownCountry(t, err, "XX")
	t.Logf("Error() = %q", err.Error())
	if err.Error() != `geo: unknown country "XX"` {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestUnknownCountryError_空码文案(t *testing.T) {
	err := &UnknownCountryError{}
	assertUnknownCountry(t, err, "")
	t.Logf("Error() = %q", err.Error())
	if err.Error() != "geo: country code is empty" {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestUnknownCountryError_nil接收者(t *testing.T) {
	got := (*UnknownCountryError)(nil).Error()
	t.Logf("(*UnknownCountryError)(nil).Error() = %q", got)
	if got != "geo: unknown country <nil>" {
		t.Errorf("Error() = %q", got)
	}
}

func TestUnknownCountryError_包装后仍可As(t *testing.T) {
	wrapped := fmt.Errorf("derive locale: %w", &UnknownCountryError{Country: "XX"})
	t.Logf("wrapped = %v", wrapped)
	assertUnknownCountry(t, wrapped, "XX")
}

func TestInvalidExtraLanguageCountError_字段与文案(t *testing.T) {
	err := &InvalidExtraLanguageCountError{Extra: -3}
	assertInvalidExtra(t, err, -3)
	want := "geo: extra language count must be non-negative: extra=-3"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestInvalidExtraLanguageCountError_nil接收者(t *testing.T) {
	got := (*InvalidExtraLanguageCountError)(nil).Error()
	t.Logf("(*InvalidExtraLanguageCountError)(nil).Error() = %q", got)
	if got != "geo: extra language count is invalid <nil>" {
		t.Errorf("Error() = %q", got)
	}
}

func TestInvalidExtraLanguageCountError_包装后仍可As(t *testing.T) {
	wrapped := fmt.Errorf("build header: %w", &InvalidExtraLanguageCountError{Extra: -1})
	t.Logf("wrapped = %v", wrapped)
	assertInvalidExtra(t, wrapped, -1)
}

func TestExtraLanguageCountExceedsPoolError_字段与文案(t *testing.T) {
	err := &ExtraLanguageCountExceedsPoolError{Extra: 10, Available: 3}
	assertExtraExceedsPool(t, err, 10, 3)
	want := "geo: extra language count exceeds candidate pool: extra=10 available=3"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestExtraLanguageCountExceedsPoolError_Available为零(t *testing.T) {
	err := &ExtraLanguageCountExceedsPoolError{Extra: 1, Available: 0}
	assertExtraExceedsPool(t, err, 1, 0)
	t.Logf("Error() = %q", err.Error())
}

func TestExtraLanguageCountExceedsPoolError_nil接收者(t *testing.T) {
	got := (*ExtraLanguageCountExceedsPoolError)(nil).Error()
	t.Logf("(*ExtraLanguageCountExceedsPoolError)(nil).Error() = %q", got)
	if got != "geo: extra language count exceeds candidate pool <nil>" {
		t.Errorf("Error() = %q", got)
	}
}

func TestExtraLanguageCountExceedsPoolError_包装后仍可As(t *testing.T) {
	wrapped := fmt.Errorf("sample prefs: %w", &ExtraLanguageCountExceedsPoolError{Extra: 8, Available: 2})
	t.Logf("wrapped = %v", wrapped)
	assertExtraExceedsPool(t, wrapped, 8, 2)
}

func TestGeo错误类型互不误匹配(t *testing.T) {
	unknown := &UnknownCountryError{Country: "XX"}
	invalid := &InvalidExtraLanguageCountError{Extra: -1}
	exceeds := &ExtraLanguageCountExceedsPoolError{Extra: 9, Available: 1}

	var asUnknown *UnknownCountryError
	var asInvalid *InvalidExtraLanguageCountError
	var asExceeds *ExtraLanguageCountExceedsPoolError

	if errors.As(invalid, &asUnknown) || errors.As(exceeds, &asUnknown) {
		t.Fatal("extra 错误不应被 As 成 *UnknownCountryError")
	}
	if errors.As(unknown, &asInvalid) || errors.As(exceeds, &asInvalid) {
		t.Fatal("非 extra 负数错误不应被 As 成 *InvalidExtraLanguageCountError")
	}
	if errors.As(unknown, &asExceeds) || errors.As(invalid, &asExceeds) {
		t.Fatal("非超池错误不应被 As 成 *ExtraLanguageCountExceedsPoolError")
	}
	t.Logf("三类错误互不误匹配")
}
