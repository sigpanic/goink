import styles from "./report.module.css";
export default function WorldIllustration() {
  return (
    <svg
      className={styles["report-world"]}
      viewBox="0 0 480 380"
      aria-hidden="true"
      fill="none"
    >
      <g stroke="currentColor" strokeWidth="1">
        <ellipse cx="258" cy="255" rx="161" ry="64" opacity=".2" />
        <ellipse cx="258" cy="255" rx="130" ry="48" opacity=".18" />
        <ellipse cx="258" cy="255" rx="101" ry="34" opacity=".16" />
        <circle cx="258" cy="174" r="127" strokeDasharray="2 7" opacity=".3" />
        <path
          d="m113 245 60-84 37 44 59-107 42 75 21-26 70 107"
          strokeWidth="1.5"
          opacity=".8"
        />
        <path
          d="m174 161 17 63 19-19 28 53 31-160-8 96 33 55 17-76 21 77"
          opacity=".4"
        />
        <path
          d="M113 245q75 42 145 27t144-18l-71 48-82 27-80-27Z"
          fill="currentColor"
          fillOpacity=".06"
          strokeOpacity=".5"
        />
        <path d="m195 250 45 15 26-8 38 14-32 18 18 15-41 25" opacity=".7" />
        <path
          d="m145 128 83-30 41-38 72 32 32 60"
          strokeDasharray="3 5"
          opacity=".4"
        />
        <path
          d="M153 239v-23l12-7 12 7v23m-27-20h30m-19 20v-10h8v10M323 249v-22l11-6 11 6v22m-26-20h30m-18 20v-10h6v10"
          opacity=".8"
        />
        <path
          d="M210 255v-17m-8 4 8-13 8 13m-15 5 7-13 7 13M362 254v-21m-9 4 9-15 9 15m-17 6 8-14 8 14"
          opacity=".7"
        />
        <circle cx="146" cy="128" r="3" fill="currentColor" />
        <circle cx="269" cy="60" r="3" fill="currentColor" />
        <circle cx="373" cy="152" r="3" fill="currentColor" />
        <path
          d="m113 100 3-7 3 7 7 3-7 3-3 7-3-7-7-3Zm235-44 2-5 2 5 5 2-5 2-2 5-2-5-5-2Z"
          opacity=".6"
        />
        <path d="M101 292h32m-16-16v32M383 101h14m-7-7v14" opacity=".25" />
      </g>
    </svg>
  );
}
