import { redirect } from "next/navigation";

/** Visual QA used a fixture route during the redesign; it is not shipped. */
export default function PreviewPage() {
  redirect("/");
}