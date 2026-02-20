import type { Metadata } from "next";
import "./globals.css";
import { TopNav } from "../components/nav";

export const metadata: Metadata = {
  title: "LitFlow",
  description: "AI literature survey engine"
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <TopNav />
        {children}
      </body>
    </html>
  );
}
