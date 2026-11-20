import type { Metadata, Viewport } from "next";
import { Geist, Geist_Mono, Instrument_Serif } from "next/font/google";
import "./globals.css";
import { ThemeProvider, themeScript } from "@/lib/providers/theme-provider";
import { QueryProvider } from "@/lib/providers/query-provider";
import { SessionProvider } from "@/lib/providers/session-provider";
import { RealtimeProvider } from "@/lib/providers/realtime-provider";
import { AppToaster } from "@/components/chrome/toaster";
import { AppFrame } from "@/components/chrome/app-frame";

/**
 * Three families, all self-hosted by next/font — no runtime Google request.
 *
 *   Instrument Serif   display — wordmark, hero, page titles, empty states
 *   Geist              UI chrome, post copy, comments, controls
 *   Geist Mono         handles, counts, timestamps, tags
 *
 * Instrument Serif is a high-contrast editorial face with a single 400 cut, so
 * display headings use its natural weight rather than a synthesised bold.
 * Metrics are matched against the fallbacks, keeping CLS at zero.
 */
const geist = Geist({
  subsets: ["latin"],
  display: "swap",
  variable: "--font-geist-sans",
  adjustFontFallback: true,
});

const geistMono = Geist_Mono({
  subsets: ["latin"],
  display: "swap",
  variable: "--font-geist-mono",
  adjustFontFallback: true,
});

const instrumentSerif = Instrument_Serif({
  subsets: ["latin"],
  display: "swap",
  variable: "--font-instrument",
  weight: "400",
  adjustFontFallback: true,
});

export const metadata: Metadata = {
  title: {
    default: "SocialFlow",
    template: "%s · SocialFlow",
  },
  description:
    "A chronological social network for engineers: posts, threads, follows, and direct messages.",
  applicationName: "SocialFlow",
  openGraph: {
    title: "SocialFlow",
    description:
      "A chronological social network for engineers: posts, threads, follows, and direct messages.",
    type: "website",
  },
  robots: { index: true, follow: true },
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  themeColor: "#000000",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html
      lang="en"
      data-theme="dark"
      suppressHydrationWarning
      className={`${geist.variable} ${geistMono.variable} ${instrumentSerif.variable}`}
    >
      <head>
        {/* Runs before paint: no flash of the wrong theme, no hydration mismatch. */}
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body>
        <ThemeProvider>
          <QueryProvider>
            <SessionProvider>
              <RealtimeProvider>
                <AppFrame>{children}</AppFrame>
              </RealtimeProvider>
              <AppToaster />
            </SessionProvider>
          </QueryProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}