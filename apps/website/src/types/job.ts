import type {
  IndustryField,
  JobType,
  LocationType,
  WorkingRight,
} from "./job.gen";

export type {
  IndustryField,
  JobType,
  LocationType,
  WorkingRight,
} from "./job.gen";

/*
 * The enum types come from schema/job.schema.json through job.gen.ts. The
 * lists below are the values the filter UI offers, which is a subset: the
 * scraper never stores EOI, and APAC has no filter of its own.
 */
export const JOB_TYPES: readonly JobType[] = [
  "PRE_PENULTIMATE",
  "INTERN",
  "GRADUATE",
  "OTHER",
];

export const LOCATIONS: readonly LocationType[] = [
  "VIC",
  "NSW",
  "QLD",
  "WA",
  "NT",
  "SA",
  "ACT",
  "TAS",
  "AUSTRALIA",
  "OTHERS",
];

export { WORKING_RIGHT_VALUES as WORKING_RIGHTS } from "./job.gen";

export { INDUSTRY_FIELD_VALUES as INDUSTRY_FIELDS } from "./job.gen";

export interface Company {
  name: string;
  website?: string | null;
  logo?: string | null;
}

export interface Job {
  id: string;
  title: string;
  description?: string | null;
  one_liner?: string | null;
  company: Company;
  application_url?: string | null;
  source_urls: string[];
  type?: JobType | null;
  close_date?: string;
  locations: LocationType[];
  industry_field?: IndustryField;
  working_rights: WorkingRight[];
  created_at: string;
  updated_at: string;
  is_sponsored: boolean;
  highlight: boolean;
}
