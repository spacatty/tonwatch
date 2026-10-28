import type { Metadata } from "next";
import { JetBrains_Mono, Space_Grotesk } from "next/font/google";
import { Providers } from "@/components/providers";
import "./globals.css";

const ui = Space_Grotesk({
  variable: "--font-ui",
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
});

const data = JetBrains_Mono({
  variable: "--font-data",
  subsets: ["latin"],
  weight: ["400", "500", "600"],
});

// Icons, the manifest and the link-preview image are the file conventions in this folder.
export const metadata: Metadata = {
  metadataBase: process.env.PUBLIC_URL ? new URL(process.env.PUBLIC_URL) : undefined,
  title: "BidSpot",
  description: "Fragment username tracker: auctions, sales and on-chain listings, LLM-filtered",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className={`${ui.variable} ${data.variable} h-full antialiased`} suppressHydrationWarning>
      <body className="min-h-full">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
