package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
)

// Referral rewards (env-tunable):
//   REFERRAL_REWARD_DAYS   premium days for the inviter per activated friend (default 3)
//   REFERRAL_WELCOME_DAYS  premium days for the invited friend (default 1)
//   REFERRAL_MONTHLY_CAP   max rewarded friends per inviter per month (default 10)
func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v >= 0 {
		return v
	}
	return def
}

func ReferralRewardDays() int  { return envInt("REFERRAL_REWARD_DAYS", 3) }
func ReferralWelcomeDays() int { return envInt("REFERRAL_WELCOME_DAYS", 1) }
func referralMonthlyCap() int  { return envInt("REFERRAL_MONTHLY_CAP", 10) }

// referralActivationWindow: a friend must start watching within this long
// after signing up for the referral to count.
const referralActivationWindow = 30 * 24 * time.Hour

// StartReferralRewardJob rewards referrals whose invited user became active
// (watched something). Runs every 10 minutes.
func StartReferralRewardJob(ctx context.Context, repo *repositories.ReferralRepository, users *repositories.UserRepository, notifications *NotificationService) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		processReferrals(ctx, repo, users, notifications)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func processReferrals(ctx context.Context, repo *repositories.ReferralRepository, users *repositories.UserRepository, notifications *NotificationService) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	activated, err := repo.ActivatedPending(cctx, referralActivationWindow, 200)
	if err != nil {
		log.Printf("[REFERRAL] load pending: %v", err)
		return
	}
	rewardDays, welcomeDays, monthlyCap := ReferralRewardDays(), ReferralWelcomeDays(), int64(referralMonthlyCap())

	for _, p := range activated {
		referrerDays := rewardDays
		status := "rewarded"
		if repo.RewardedThisMonth(cctx, p.ReferrerID) >= monthlyCap {
			referrerDays = 0
			status = "capped" // friend still gets the welcome bonus
		}
		// Claim the row first so a concurrent run can't double-reward.
		ok, err := repo.MarkDone(cctx, p.ID, status, referrerDays, welcomeDays)
		if err != nil || !ok {
			continue
		}
		if referrerDays > 0 {
			if exp, err := users.ActivateOrExtendPremium(p.ReferrerID, referrerDays); err == nil && notifications != nil {
				notifications.NotifyUserBoth(cctx, p.ReferrerID, models.NotificationReferralReward,
					"Do'stingiz qo'shildi — premium sovg'a!",
					fmt.Sprintf("Taklif qilgan do'stingiz tomosha qilishni boshladi. Sizga %d kun premium qo'shildi (%s gacha).", referrerDays, exp.Format("02.01.2006")),
					"/user",
					map[string]interface{}{"days": referrerDays, "referred_id": p.ReferredID.Hex()},
					fmt.Sprintf("🎁 Taklif qilgan do'stingiz FilmoraUz'da tomosha qilishni boshladi!\nSizga <b>%d kun premium</b> qo'shildi.", referrerDays),
					"👑 Profilim")
			}
		}
		if welcomeDays > 0 {
			if _, err := users.ActivateOrExtendPremium(p.ReferredID, welcomeDays); err == nil && notifications != nil {
				notifications.NotifyUserBoth(cctx, p.ReferredID, models.NotificationReferralReward,
					"Xush kelibsiz sovg'asi",
					fmt.Sprintf("Do'stingiz taklifi uchun sizga %d kun premium berildi. Maroqli tomosha!", welcomeDays),
					"/premium",
					map[string]interface{}{"days": welcomeDays},
					"", "")
			}
		}
		log.Printf("[REFERRAL] %s: referrer=%s +%dd, referred=%s +%dd", status, p.ReferrerID.Hex(), referrerDays, p.ReferredID.Hex(), welcomeDays)
	}
}
