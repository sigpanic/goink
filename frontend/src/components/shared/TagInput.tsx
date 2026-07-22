import { useState, type KeyboardEvent } from "react";
import { X } from "lucide-react";

interface Props {
  tags: string[];
  onChange: (tags: string[]) => void;
  placeholder?: string;
  size?: "sm" | "md";
}

export default function TagInput({
  tags,
  onChange,
  placeholder,
  size = "sm",
}: Props) {
  const [input, setInput] = useState("");

  const isMd = size === "md";

  function addTag(value: string) {
    const v = value.trim();
    if (!v || tags.includes(v)) return;
    onChange([...tags, v]);
  }

  function removeTag(index: number) {
    onChange(tags.filter((_, i) => i !== index));
  }

  function handleKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter") {
      e.preventDefault();
      addTag(input);
      setInput("");
    } else if (e.key === "Backspace" && !input && tags.length > 0) {
      removeTag(tags.length - 1);
    }
  }

  return (
    <div
      className={`flex flex-wrap items-center gap-1 rounded-md border border-border bg-background px-2 ${isMd ? "py-1.5 min-h-[36px]" : "py-1 min-h-[30px]"} cursor-text focus-within:ring-2 focus-within:ring-ring`}
      onClick={() => document.getElementById("tag-input-field")?.focus()}
    >
      {tags.map((tag, i) => (
        <span
          key={i}
          className={`inline-flex items-center gap-0.5 rounded px-1.5 ${isMd ? "py-0.5 text-sm" : "py-0.5 text-xs"} font-medium bg-secondary text-secondary-foreground`}
        >
          {tag}
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              removeTag(i);
            }}
            className="rounded-full p-0.5 hover:bg-muted transition-colors"
          >
            <X className={isMd ? "h-3 w-3" : "h-2.5 w-2.5"} />
          </button>
        </span>
      ))}
      <input
        id="tag-input-field"
        type="text"
        value={input}
        onChange={(e) => setInput(e.target.value)}
        onKeyDown={handleKeyDown}
        onBlur={() => {
          if (input.trim()) {
            addTag(input);
            setInput("");
          }
        }}
        placeholder={tags.length === 0 ? placeholder : ""}
        className={`flex-1 min-w-[80px] bg-transparent border-none outline-none ${isMd ? "text-sm" : "text-xs"} text-foreground placeholder:text-muted-foreground ${isMd ? "py-1" : "py-0.5"}`}
      />
    </div>
  );
}
