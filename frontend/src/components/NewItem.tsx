"use client";
// Inline "create" card for list pages: name first, key derived from it.
import { useState } from "react";
import { t } from "@/lib/i18n";
import { Field } from "./ui";

const slug = (s: string) => s.toLowerCase().normalize("NFKD").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");

export default function NewItem({ onCreate, onCancel, label }: { onCreate: (key: string, name: string) => void; onCancel: () => void; label: string }) {
  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const [touched, setTouched] = useState(false);
  const k = touched ? key : slug(name);
  return (
    <section className="card" style={{ marginBottom: 16 }}>
      <h3>{label}</h3>
      <div className="row3">
        <Field label={t("Name")}><input autoFocus value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label={t("Key (lowercase)")}><input className="mono" value={k} onChange={(e) => { setTouched(true); setKey(slug(e.target.value)); }} /></Field>
      </div>
      <div className="bar"><button className="btn" disabled={!name || !k} onClick={() => onCreate(k, name)}>{t("Create")}</button><button className="link" onClick={onCancel}>{t("Cancel")}</button></div>
    </section>
  );
}
