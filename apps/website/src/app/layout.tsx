// Order seems to matter. If Mantine is imported after tailwind,
// the tailwind class passed with className is not applied.
import "@mantine/core/styles.css";
import "./globals.css";
import "@mantine/notifications/styles.css";

import { GoogleAnalytics } from "@next/third-parties/google";
import Link from "next/link";

import NavBar from "@/components/layout/nav-bar";
import { MantineProvider, v8CssVariablesResolver } from "@mantine/core";
import { ColorSchemeScript } from "@mantine/core";
import { type PropsWithChildren, Suspense } from "react";
import Head from "next/head";
import { theme } from "@/lib/theme";

import { Poppins } from "next/font/google";
import { FilterProvider } from "@/context/filter/filter-provider";
import type { Metadata } from "next";
import FeedbackButton from "@/components/ui/feedback-button";
import { Notifications } from "@mantine/notifications";

import FirstVisitNotification from "@/components/ui/first-visit-notification";
import { Analytics } from "@/components/analytics/analytics";

export const metadata: Metadata = {
  title: {
    template: "%s | MAC Jobs Board",
    default: "MAC Jobs Board",
  },
  openGraph: {
    title: "MAC Jobs Board",
    description: "Stay ahead with the job board that never sleeps.",
    images: [
      {
        url: "/OgImage.png",
        alt: "MAC Jobs Board",
      },
    ],
  },
};

const poppins = Poppins({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
});

export default function RootLayout({ children }: PropsWithChildren) {
  return (
    <html lang="en" data-mantine-color-scheme="dark">
      <Head>
        <ColorSchemeScript defaultColorScheme="dark" />
      </Head>
      <body className={`${poppins.className}`}>
        <Suspense>
          <MantineProvider
            theme={theme}
            defaultColorScheme="dark"
            cssVariablesResolver={v8CssVariablesResolver}
          >
            <FilterProvider>
              <div className="min-h-screen flex flex-col px-6">
                <Notifications
                  className="mploy-notifications"
                  pauseResetOnHover="notification"
                />
                <NavBar />
                <main className="">
                  {children}
                  <FirstVisitNotification />
                  <FeedbackButton />
                  <footer className="fixed bottom-5 left-6 z-40 text-xs font-semibold text-white/45">
                    <Link
                      className="transition-colors hover:text-accent focus-visible:text-accent focus-visible:outline-none"
                      href="/privacy"
                    >
                      Privacy
                    </Link>
                  </footer>
                  <Analytics />
                  <GoogleAnalytics gaId="G-1RXLVCFJC0" />
                </main>
              </div>
            </FilterProvider>
          </MantineProvider>
        </Suspense>
      </body>
    </html>
  );
}
