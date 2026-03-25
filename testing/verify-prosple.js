const { chromium } = require("playwright");

const PROSPLE_URL = "https://prosple-gw.global.ssl.fastly.net/internal";

const QUERY = `query OpportunitiesSearch($parameters: OpportunitiesSearchInput!) {
  opportunitiesSearch(parameters: $parameters) {
    opportunities {
      id
      title
      applyByUrl
      detailPageURL
      overview {
        fullText
      }
      opportunityTypes {
        label
      }
      applicationsOpenDate
      applicationsCloseDate
      physicalLocations {
        label
        children { label }
      }
      parentEmployer {
        title
        detailPageURL
        logo { thumbnail { url } }
      }
      workingRights {
        children { label }
      }
    }
  }
}`;

function buildVariables(offset, field) {
  return {
    parameters: {
      gid: "6",
      range: { offset, limit: 5 },
      sortBy: { criteria: "NEWEST_OPPORTUNITIES", direction: "DESC" },
      defaultBid: 0,
      selectedStartDateRangeFacet: null,
      experiments: {
        sortByBidPopularityKeyword: "FIFTH_VARIATION",
        prod2432: false,
      },
      locationFilter: { location: "Australia" },
      unitOfDistance: "KM",
      workRightLocation: "29058",
      selectedOpportunityTypeFacets: [
        { id: "1", childFacets: [] },
        { id: "2", childFacets: [] },
        { id: "9", childFacets: [] },
      ],
      studyFieldsFilter: field,
    },
  };
}

async function verifyProsple() {
  const browser = await chromium.launch();
  const context = await browser.newContext();
  const request = context.request;

  let passed = 0;
  let failed = 0;

  function check(label, condition, detail = "") {
    if (condition) {
      console.log(`  ✓ ${label}${detail ? " — " + detail : ""}`);
      passed++;
    } else {
      console.error(`  ✗ ${label}${detail ? " — " + detail : ""}`);
      failed++;
    }
  }

  // Test each study field
  for (const field of ["502", "532", "677"]) {
    const fieldName = { "502": "IT & CS", "532": "Engineering", "677": "Maths" }[field];
    console.log(`\n── Field ${field} (${fieldName}) ──`);

    const response = await request.post(PROSPLE_URL, {
      headers: { "Content-Type": "application/json" },
      data: {
        operationName: "OpportunitiesSearch",
        query: QUERY,
        variables: buildVariables(0, field),
      },
    });

    check("HTTP 200", response.status() === 200, `got ${response.status()}`);

    const body = await response.json();

    check("No GraphQL errors", !body.errors, body.errors ? body.errors[0]?.message : "");
    check("Has data", !!body.data?.opportunitiesSearch?.opportunities);

    const jobs = body.data?.opportunitiesSearch?.opportunities ?? [];
    check("Returns jobs", jobs.length > 0, `got ${jobs.length}`);

    if (jobs.length > 0) {
      const withDesc = jobs.filter(j => j.overview?.fullText?.trim().length > 0);
      const withTitle = jobs.filter(j => j.title?.trim().length > 0);
      const withApplyUrl = jobs.filter(j => j.applyByUrl?.trim().length > 0);

      check(
        "Jobs have descriptions",
        withDesc.length > 0,
        `${withDesc.length}/${jobs.length} have overview.fullText`
      );
      check("Jobs have titles", withTitle.length === jobs.length, `${withTitle.length}/${jobs.length}`);
      check("Jobs have apply URLs", withApplyUrl.length > 0, `${withApplyUrl.length}/${jobs.length}`);

      // Print a sample
      const sample = jobs[0];
      console.log(`\n  Sample job:`);
      console.log(`    Title:       ${sample.title}`);
      console.log(`    Company:     ${sample.parentEmployer?.title}`);
      console.log(`    Description: ${sample.overview?.fullText?.slice(0, 80)?.trim()}...`);
      console.log(`    Apply URL:   ${sample.applyByUrl}`);
    }
  }

  await browser.close();

  console.log(`\n──────────────────────────`);
  console.log(`Results: ${passed} passed, ${failed} failed`);
  if (failed > 0) process.exit(1);
}

verifyProsple().catch(err => {
  console.error("Fatal:", err.message);
  process.exit(1);
});
