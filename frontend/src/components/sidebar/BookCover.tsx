import { useState } from "react";
import defaultCover from "@/assets/covers/default-cover.jpg";

interface Props {
  novelId?: number;
  refreshKey?: number;
}

export default function BookCover({ novelId, refreshKey }: Props) {
  const [errored, setErrored] = useState(false);

  const src =
    novelId && !errored
      ? `/covers/${novelId}?v=${refreshKey ?? 0}`
      : defaultCover;

  return (
    <div className="w-full aspect-[3/4] rounded-md overflow-hidden shadow-sm select-none relative bg-muted">
      <img
        key={refreshKey}
        src={src}
        alt=""
        onError={() => setErrored(true)}
        className="w-full h-full object-cover block"
      />
    </div>
  );
}
