import type { Metadata } from "next";
import Link from "next/link";
import { Send } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";

export const metadata: Metadata = {
  title: "Privacy Policy — FILMORAUZ",
  description:
    "FilmoraUz Privacy Policy: what information we collect, how we use it, which third parties are involved and how you can control your data.",
  alternates: { canonical: "/privacy" },
  robots: { index: true, follow: true },
};

const LAST_UPDATED = "October 5, 2026";

export default function PrivacyPage() {
  return (
    <>
      <Navbar />
      <main lang="en" className="min-h-screen pt-20 sm:pt-24">
        <div className="max-w-3xl mx-auto px-4 py-10 sm:py-14">
          <h1 className="font-display text-3xl sm:text-4xl md:text-5xl text-white tracking-wider mb-6">
            Privacy Policy
          </h1>
          <p className="text-gray-400 text-sm sm:text-base mb-2">
            How FilmoraUz collects, uses and protects your information.
          </p>
          <p className="text-gray-500 text-xs sm:text-sm mb-10">
            Last updated: {LAST_UPDATED}
          </p>

          <div className="space-y-6 text-gray-300 text-sm sm:text-base leading-relaxed">
            <section className="glass-card border border-white/10 rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                1. Overview
              </h2>
              <p>
                This Privacy Policy explains what information FilmoraUz
                (&quot;we&quot;, &quot;us&quot;, &quot;the service&quot;) collects when you use
                our website and Telegram bot, why we collect it and what choices
                you have. By using the service you agree to the practices
                described here. If you do not agree, please stop using the
                service.
              </p>
            </section>

            <section className="glass-card border border-white/10 rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                2. Information we collect
              </h2>
              <p className="mb-3">
                You can browse the catalog without an account. If you sign in or
                interact with the service, we collect the following:
              </p>
              <ul className="list-disc pl-5 space-y-1.5 text-gray-300">
                <li>
                  <span className="text-white">Telegram account data.</span>{" "}
                  Sign-in works only through our Telegram bot. When you sign in
                  we receive your Telegram ID, chat ID, first and last name,
                  username, profile photo and language code. We never receive
                  your Telegram password or phone number.
                </li>
                <li>
                  <span className="text-white">Profile data.</span> The display
                  name, avatar, profile style and privacy settings you choose.
                </li>
                <li>
                  <span className="text-white">Activity data.</span> Watch
                  history and playback progress, favorites, lists, ratings,
                  reviews, comments, content suggestions, watch-room messages
                  and notification preferences.
                </li>
                <li>
                  <span className="text-white">Account status.</span> Premium
                  subscription dates, wallet balance, referral information and,
                  where applicable, ban records and appeals.
                </li>
                <li>
                  <span className="text-white">Technical data.</span> IP
                  address, browser user agent, pages and titles viewed, share
                  link visits, ad impressions and clicks, and error reports.
                </li>
              </ul>
            </section>

            <section className="glass-card border border-white/10 rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                3. How we use your information
              </h2>
              <ul className="list-disc pl-5 space-y-1.5 text-gray-300">
                <li>To sign you in and keep your session active</li>
                <li>
                  To provide features such as continue watching, favorites,
                  lists, comments and watch rooms
                </li>
                <li>To recommend movies and series you may like</li>
                <li>To manage premium subscriptions and referral rewards</li>
                <li>
                  To send notifications you have enabled, on the website or
                  through Telegram
                </li>
                <li>To show ads and measure how they perform</li>
                <li>
                  To understand how the service is used, fix errors and improve
                  it
                </li>
                <li>
                  To prevent abuse, moderate content and enforce our rules
                </li>
              </ul>
            </section>

            <section className="glass-card border border-white/10 rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                4. Cookies and local storage
              </h2>
              <p>
                We use a cookie named <code className="text-white">auth_token</code>{" "}
                to keep you signed in. We also use your browser&apos;s local
                storage to remember preferences such as player settings,
                playback position, recent searches and dismissed banners. Google
                Analytics sets its own cookies to measure site traffic. You can
                clear or block cookies in your browser settings, but sign-in
                will not work without the authentication cookie.
              </p>
            </section>

            <section className="glass-card border border-white/10 rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                5. Third-party services
              </h2>
              <p className="mb-3">
                We do not sell your personal information. We rely on a small
                number of third parties to run the service, and they process
                data under their own privacy policies:
              </p>
              <ul className="list-disc pl-5 space-y-1.5 text-gray-300">
                <li>
                  <span className="text-white">Telegram</span> — sign-in, bot
                  messages and notifications
                </li>
                <li>
                  <span className="text-white">Google Analytics</span> —
                  aggregated traffic statistics
                </li>
                <li>
                  <span className="text-white">Hosting and content delivery providers</span>{" "}
                  — storing and delivering video, images and site data
                </li>
                <li>
                  <span className="text-white">Advertisers</span> — ads shown on
                  the site may link to external websites that we do not control
                </li>
              </ul>
              <p className="mt-3">
                We may also disclose information when required by law or when
                necessary to protect the service and its users.
              </p>
            </section>

            <section className="glass-card border border-white/10 rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                6. Information visible to others
              </h2>
              <p>
                Your display name, avatar, comments, reviews, ratings and public
                lists can be seen by other visitors. Messages you send in a
                watch room are visible to its participants. You can make your
                profile private in your profile settings to limit what others
                see.
              </p>
            </section>

            <section className="glass-card border border-white/10 rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                7. Data retention and security
              </h2>
              <p>
                We keep your information for as long as your account exists or
                as needed to operate the service and meet legal obligations. We
                apply reasonable technical and organizational measures to
                protect it, but no method of transmission or storage on the
                internet is completely secure, and we cannot guarantee absolute
                security.
              </p>
            </section>

            <section className="glass-card border border-white/10 rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                8. Your choices and rights
              </h2>
              <ul className="list-disc pl-5 space-y-1.5 text-gray-300">
                <li>Edit your display name and profile in your account</li>
                <li>Remove favorites, lists and your own comments</li>
                <li>Turn notifications on or off at any time</li>
                <li>Sign out to remove the authentication cookie</li>
                <li>
                  Ask us for a copy of your data, a correction, or deletion of
                  your account and associated data
                </li>
              </ul>
              <p className="mt-3">
                To make a request, contact us using the details below. We may
                need to verify that the request comes from the account owner.
              </p>
            </section>

            <section className="glass-card border border-white/10 rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                9. Children
              </h2>
              <p>
                The service is not directed at children under 13, and we do not
                knowingly collect their personal information. If you believe a
                child has provided us with personal data, contact us and we will
                remove it.
              </p>
            </section>

            <section className="glass-card border border-white/10 rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                10. Changes to this policy
              </h2>
              <p>
                We may update this Privacy Policy from time to time. The latest
                version is always available on this page, with the date of the
                last update shown at the top. Continued use of the service after
                a change means you accept the updated policy.
              </p>
            </section>

            <section className="glass-card border border-white/10 rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-3">
                11. Contact
              </h2>
              <p className="mb-3">
                Questions and requests about your privacy are accepted through
                Telegram. For copyright matters, see our{" "}
                <Link href="/dmca" className="text-brand-red hover:underline">
                  DMCA
                </Link>{" "}
                page.
              </p>
              <Link
                href="https://t.me/filmorauznet?direct"
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-2 bg-brand-red hover:bg-orange-700 text-white font-semibold px-5 py-2.5 rounded-lg transition-colors"
              >
                <Send size={16} />
                Telegram
              </Link>
            </section>
          </div>
        </div>
      </main>
      <Footer />
    </>
  );
}
