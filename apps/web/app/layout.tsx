import type { Metadata, Viewport } from "next";
import localFont from "next/font/local";
import type { ReactNode } from "react";

import "./globals.css";

/** Outfit (SIL OFL), self-hosted so sign-in needs no third-party font fetch. */
const outfit = localFont({
  src: "./fonts/outfit-latin.woff2",
  weight: "100 900",
  display: "swap",
  variable: "--font-outfit",
});

export const metadata: Metadata = {
  title: "Atlas · SafariStack",
  description:
    "Atlas - the SafariStack Company Operating System: strategy, work, evidence and decisions in one place.",
};

export const viewport: Viewport = {
  themeColor: "#FBF8F1",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" className={outfit.variable}>
      <body className="min-h-screen bg-milk font-sans text-slate-900 antialiased">
        {children}
      </body>
    </html>
  );
}
