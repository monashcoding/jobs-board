"""The bot's view of a listing must agree with schema/job.schema.json."""

from __future__ import annotations

from src.backend.mongo.collections import job_schema
from src.backend.mongo.collections.col_jobs import JobDocument, job_col
from src.core.functions import job_tags


def _stored_name(field: str) -> str:
    """Return the name a JobDocument field has in MongoDB."""
    return JobDocument.model_fields[field].alias or field


def test_job_document_only_names_schema_fields() -> None:
    stored = {_stored_name(field) for field in JobDocument.model_fields}

    assert stored <= set(job_schema.LISTING_FIELDS)


def test_job_collection_is_the_schema_collection() -> None:
    assert job_col.collection_name == job_schema.LISTING_COLLECTION


def test_tag_tables_only_name_schema_values() -> None:
    assert set(job_tags._TYPE_TO_TAG) <= set(job_schema.JOB_TYPE_VALUES)
    assert set(job_tags._LOCATION_TO_TAG) <= set(job_schema.LOCATION_TYPE_VALUES)
    assert set(job_tags._RIGHTS_TO_TAG) <= set(job_schema.WORKING_RIGHT_VALUES)
    assert job_tags._ANY_RIGHTS_REQUIRES <= set(job_schema.WORKING_RIGHT_VALUES)
